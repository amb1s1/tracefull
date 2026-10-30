// Package collect orchestrates attributed read-only device observations.
package collect

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/amb1s1/tracefull/internal/app"
	"github.com/amb1s1/tracefull/internal/config"
	"github.com/amb1s1/tracefull/internal/device"
	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/driver/cumulus"
	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/report"
	"github.com/amb1s1/tracefull/internal/trace"
	"github.com/amb1s1/tracefull/internal/verdict"
)

type Options struct {
	Events                                         chan<- Event
	From, Inventory, Destination, EvidenceDir, VRF string
	User, Jump, KnownHosts, Wrapper                string
	Identities, Groups, Maps, Extras               []string
	Sudo, InsecureHostKey, OwnerSweep              bool
	Site, Role                                     string
	Workers                                        int
	Timeout, ConnectTimeout, CommandTimeout        time.Duration
}

func Defaults() Options {
	return Options{Workers: 4, Timeout: 5 * time.Minute, ConnectTimeout: 5 * time.Second, CommandTimeout: 25 * time.Second, Groups: []string{"ingress", "route", "counters", "lldp", "version"}}
}

// Registry is explicit compile-time composition. No unvalidated platform gets
// a fallback policy, including for extra commands.
func Registry(platform string) (driver.Driver, bool) {
	switch platform {
	case "nvidia-cumulus", "cumulus":
		return cumulus.Driver{}, true
	}
	return nil, false
}

type Transport struct {
	Kind            string `json:"kind"`
	Address         string `json:"address"`
	User            string `json:"user"`
	Jump            string `json:"jump"`
	KnownHosts      string `json:"known_hosts"`
	InsecureHostKey bool   `json:"insecure_host_key"`
}
type PlannedCommand struct {
	driver.Command
	Argv []string `json:"argv"`
}
type DevicePlan struct {
	Device       inventory.Device `json:"device"`
	HopAddresses []string         `json:"hop_addresses"`
	Transport    Transport        `json:"transport"`
	Commands     []PlannedCommand `json:"commands"`
	Conditional  []PlannedCommand `json:"conditional_commands"`
	Status       string           `json:"status"`
	Reason       string           `json:"reason"`
}
type Plan struct {
	Destination  string       `json:"destination"`
	Context      string       `json:"context"`
	Devices      []DevicePlan `json:"devices"`
	Notes        []string     `json:"notes"`
	SweepDevices []DevicePlan `json:"sweep_devices"`
}
type Prepared struct {
	Trace   report.Report
	Plan    Plan
	Raw     []byte
	Options Options
}

func Prepare(ctx context.Context, o Options, stdin io.ReadCloser) (Prepared, error) {
	p := Prepared{Options: o}
	if o.Workers < 1 || o.Workers > 32 || o.Timeout <= 0 || o.Timeout > 30*time.Minute || o.ConnectTimeout <= 0 || o.ConnectTimeout > 5*time.Minute || o.CommandTimeout <= 0 || o.CommandTimeout > 5*time.Minute {
		return p, fmt.Errorf("workers must be 1–32, overall timeout <=30m, and connection/command timeouts positive and <=5m")
	}
	if len(o.Groups) == 1 && o.Groups[0] == "all" {
		o.Groups = []string{"ingress", "route", "counters", "optics", "lldp", "version", "cpu", "reverse"}
	}
	if len(o.Groups) == 0 {
		return p, fmt.Errorf("at least one collection group is required")
	}
	for _, g := range o.Groups {
		if !slices.Contains([]string{"ingress", "route", "counters", "optics", "lldp", "version", "cpu", "reverse"}, g) {
			return p, fmt.Errorf("unknown collection group %q", g)
		}
	}
	if len(o.Extras) > 32 {
		return p, fmt.Errorf("at most 32 extra commands are allowed")
	}
	if o.From == "" {
		return p, fmt.Errorf("--from is required")
	}
	a := app.Options{From: o.From, Inventory: o.Inventory, Dest: o.Destination, Maps: o.Maps, UserConfigDir: config.UserDir(), ProjectDir: ".", Timeout: time.Second, LatencyJump: verdict.DefaultLatencyJump}
	inv, notes, err := app.Inventory(a)
	if err != nil {
		return p, err
	}
	if len(inv.Devices) == 0 {
		return p, fmt.Errorf("collection requires a nonempty inventory")
	}
	p.Raw, err = app.ReadInput(ctx, o.From, stdin)
	if err != nil {
		return p, err
	}
	a.From = "-"
	p.Trace, err = app.BuildWithInventory(ctx, a, io.NopCloser(bytes.NewReader(p.Raw)), inv, notes)
	if err != nil {
		return p, err
	}
	p.Trace.Source.Name = o.From
	if o.User == "" {
		o.User = inv.Defaults.SSHUser
	}
	if o.User == "" {
		u, err := user.Current()
		if err != nil {
			return p, err
		}
		o.User = u.Username
	}
	if o.Jump == "" {
		o.Jump = inv.Defaults.JumpHost
	}
	if o.KnownHosts == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p, err
		}
		o.KnownHosts = filepath.Join(home, ".ssh", "known_hosts")
	}
	if o.VRF == "" {
		o.VRF = "default"
	}
	p.Options = o
	p.Plan = Plan{SweepDevices: []DevicePlan{}, Destination: p.Trace.DestAddr, Context: o.VRF, Devices: []DevicePlan{}, Notes: []string{"Interface-dependent commands use discovery results; dry-run leaves them deferred.", "Device observations are later snapshots, not proof of the saved trace's forwarding path."}}
	matches := map[string][]string{}
	add := func(hops []trace.Hop) {
		for _, h := range hops {
			for ip, m := range h.Matches {
				if m.Device != "" {
					matches[m.Device] = trace.AddUnique(matches[m.Device], ip)
				}
			}
		}
	}
	add(p.Trace.Hops)
	for _, path := range p.Trace.Paths {
		add(path.Hops)
	}
	for _, d := range inv.Devices {
		ips := matches[d.Name]
		if len(ips) > 0 {
			dp, err := devicePlan(p, d, ips, false)
			if err != nil {
				return p, err
			}
			p.Plan.Devices = append(p.Plan.Devices, dp)
		}
		if o.OwnerSweep && (o.Site == "" || d.Site == o.Site) && (o.Role == "" || d.Role == o.Role) && len(unmatched(p.Trace)) > 0 {
			dp, err := devicePlan(p, d, []string{}, true)
			if err != nil {
				return p, err
			}
			normal, e := devicePlan(p, d, []string{}, false)
			if e != nil {
				return p, e
			}
			dp.Conditional = normal.Commands
			for i := range dp.Conditional {
				if dp.Conditional[i].Status == "planned" {
					dp.Conditional[i].Status = "conditional"
					dp.Conditional[i].Reason = "only after unique owner discovery"
				}
			}
			p.Plan.SweepDevices = append(p.Plan.SweepDevices, dp)
		}
	}
	if (o.Site != "" || o.Role != "") && !o.OwnerSweep {
		return p, fmt.Errorf("--site and --role require --owner-sweep")
	}
	if len(p.Plan.SweepDevices) > 256 {
		return p, fmt.Errorf("owner sweep is limited to 256 inventory devices")
	}
	if len(p.Plan.SweepDevices) > 0 {
		p.Plan.Notes = append(p.Plan.Notes, "Owner sweep reads addresses first, closes its sessions, and collects unique matches in a second bounded pass with the same credentials. Commands for newly matched devices remain conditional on ownership.")
	}
	if len(p.Plan.Devices) > 256 {
		return p, fmt.Errorf("collection is limited to 256 matched devices per run")
	}
	slices.SortFunc(p.Plan.Devices, func(a, b DevicePlan) int { return strings.Compare(a.Device.Name, b.Device.Name) })
	return p, nil
}
func devicePlan(p Prepared, d inventory.Device, ips []string, sweep bool) (DevicePlan, error) {
	o := p.Options
	ips = slices.Clone(ips)
	slices.Sort(ips)
	dp := DevicePlan{Conditional: []PlannedCommand{}, Device: d, HopAddresses: ips, Commands: []PlannedCommand{}, Status: "planned", Transport: Transport{Kind: "native-ssh", Address: d.Address, User: o.User, Jump: o.Jump, KnownHosts: o.KnownHosts, InsecureHostKey: o.InsecureHostKey}}
	if o.Wrapper != "" {
		dp.Transport.Kind = "wrapper"
	}
	drv, ok := Registry(d.Platform)
	if !ok {
		dp.Status, dp.Reason = "unsupported", "no capture-validated driver or command policy"
		return dp, nil
	}
	groups := o.Groups
	if sweep {
		groups = []string{"ingress"}
	}
	params := parameters(p, nil)
	commands, err := drv.Plan(groups, params)
	if err != nil {
		return dp, fmt.Errorf("%s: %w", d.Name, err)
	}
	if !sweep {
		for i, extra := range o.Extras {
			if err := drv.Validate(extra, params); err != nil {
				return dp, err
			}
			commands = append(commands, driver.Command{Name: fmt.Sprintf("extra-%d", i+1), Group: "extra", Text: extra, Status: "planned"})
		}
	}
	dp.Commands, err = planned(commands, dp, o, drv, params)
	return dp, err
}
func parameters(p Prepared, interfaces []string) driver.Params {
	return driver.Params{Destination: p.Plan.Destination, Source: p.Trace.SourceAddr, VRF: p.Options.VRF, Sudo: p.Options.Sudo, Interfaces: interfaces}
}
func wrapperTarget(p DevicePlan) device.Target {
	return device.Target{Host: p.Device.Name, Address: p.Device.Address, User: p.Transport.User, Jump: p.Transport.Jump, Platform: p.Device.Platform}
}
func planned(commands []driver.Command, dp DevicePlan, o Options, drv driver.Driver, params driver.Params) ([]PlannedCommand, error) {
	out := []PlannedCommand{}
	var w *device.Wrapper
	if o.Wrapper != "" {
		var err error
		w, err = device.NewWrapper(o.Wrapper, wrapperTarget(dp), func(s string) error { return drv.Validate(s, params) }, o.CommandTimeout)
		if err != nil {
			return nil, err
		}
		defer func() { _ = w.Close() }()
	}
	for _, c := range commands {
		pc := PlannedCommand{Command: c, Argv: []string{}}
		if w != nil && c.Status == "planned" {
			var err error
			pc.Argv, err = w.Plan(c.Text)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, pc)
	}
	return out, nil
}

type Factory func(context.Context, DevicePlan, device.Policy) (device.Session, error)

func SystemFactory(o Options) Factory {
	return func(ctx context.Context, p DevicePlan, policy device.Policy) (device.Session, error) {
		if o.Wrapper != "" {
			return device.NewWrapper(o.Wrapper, wrapperTarget(p), policy, o.CommandTimeout)
		}
		address := p.Device.Address
		if prefix, err := netip.ParsePrefix(address); err == nil {
			address = prefix.Addr().String()
		}
		if _, _, err := net.SplitHostPort(address); err != nil {
			address = net.JoinHostPort(address, "22")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		s := device.SSHOptions{DefaultIdentityDir: filepath.Join(home, ".ssh"), Address: address, User: p.Transport.User, KnownHosts: p.Transport.KnownHosts, AgentSocket: os.Getenv("SSH_AUTH_SOCK"), IdentityFiles: o.Identities, InsecureHostKey: o.InsecureHostKey, ConnectTimeout: o.ConnectTimeout, CommandTimeout: o.CommandTimeout}
		if p.Transport.Jump != "" {
			jump := p.Transport.Jump
			if _, _, err := net.SplitHostPort(jump); err != nil {
				jump = net.JoinHostPort(jump, "22")
			}
			j := s
			j.Address = jump
			s.Jump = &j
		}
		return device.DialSSH(ctx, s, policy)
	}
}

func unmatched(r report.Report) []string {
	ips := []string{}
	visit := func(hops []trace.Hop) {
		for _, h := range hops {
			for ip, m := range h.Matches {
				if m.Method == "unmatched" {
					ips = trace.AddUnique(ips, ip)
				}
			}
		}
	}
	visit(r.Hops)
	for _, path := range r.Paths {
		visit(path.Hops)
	}
	slices.Sort(ips)
	return ips
}
