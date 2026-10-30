package collect

import (
	"context"
	"errors"
	"fmt"
	"github.com/amb1s1/tracefull/internal/trace"
	"maps"
	"slices"

	"sync"
	"time"

	"github.com/amb1s1/tracefull/internal/device"
	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/evidence"
	"github.com/amb1s1/tracefull/internal/report"
)

var ErrNoResults = errors.New("no device collection produced usable observations")

type CommandResult struct {
	PlannedCommand
	ElapsedSeconds *float64       `json:"elapsed_seconds"`
	StartedAt      *string        `json:"started_at"`
	EndedAt        *string        `json:"ended_at"`
	ExitCode       *int           `json:"exit_code"`
	Stdout         *evidence.File `json:"stdout"`
	Stderr         *evidence.File `json:"stderr"`
	Facts          *driver.Facts  `json:"facts"`
}
type DeviceResult struct {
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	Reason     string          `json:"reason"`
	Interfaces []string        `json:"interfaces"`
	Commands   []CommandResult `json:"commands"`
}
type Finding struct {
	Rule        string   `json:"rule"`
	Severity    string   `json:"severity"`
	Limitations []string `json:"limitations"`
	Kind        string   `json:"kind"`
	Device      string   `json:"device"`
	Text        string   `json:"text"`
	Evidence    []string `json:"evidence"`
}
type Report struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	Plan          Plan           `json:"plan"`
	Trace         report.Report  `json:"trace"`
	Source        *evidence.File `json:"source"`
	EvidenceDir   string         `json:"evidence_dir"`
	StartedAt     string         `json:"started_at"`
	EndedAt       string         `json:"ended_at"`
	Status        string         `json:"status"`
	Error         string         `json:"error"`
	Devices       []DeviceResult `json:"devices"`
	OwnerSweep    []DeviceResult `json:"owner_sweep"`
	Findings      []Finding      `json:"findings"`
}

func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func Run(ctx context.Context, p Prepared, factory Factory) (r Report, err error) {
	// Sweep updates only cloned match maps; Prepared can be reused safely.
	p.Trace.Hops = cloneMatches(p.Trace.Hops)
	p.Trace.Paths = slices.Clone(p.Trace.Paths)
	for i := range p.Trace.Paths {
		p.Trace.Paths[i].Hops = cloneMatches(p.Trace.Paths[i].Hops)
	}
	p.Plan.Devices = slices.Clone(p.Plan.Devices)
	r = Report{SchemaVersion: "1", Kind: "collection", Plan: p.Plan, Trace: p.Trace, StartedAt: stamp(), Devices: initialDevices(p.Plan.Devices), OwnerSweep: []DeviceResult{}, Findings: []Finding{}}
	if p.Options.Workers < 1 || p.Options.Timeout <= 0 {
		return r, fmt.Errorf("invalid prepared execution limits")
	}
	if factory == nil {
		return r, fmt.Errorf("session factory unavailable")
	}
	factory = serializeHosts(factory)
	ctx, cancel := context.WithTimeout(ctx, p.Options.Timeout)
	defer cancel()
	r.EvidenceDir, err = evidence.Create(p.Options.EvidenceDir, ".tracefull-collection-")
	if err != nil {
		return r, err
	}
	defer func() {
		r.EndedAt = stamp()
		if err != nil {
			r.Error = err.Error()
		}
		if ctx.Err() != nil {
			r.Status = "interrupted"
		} else if r.Status == "" {
			r.Status = "failed"
		}
		if _, writeErr := evidence.JSON(r.EvidenceDir, "report.json", r); writeErr != nil {
			err = fmt.Errorf("finalize collection: %w (execution error: %v)", writeErr, err)
		}
	}()
	r.Source, err = evidence.Write(r.EvidenceDir, "trace.input", p.Raw)
	if err != nil {
		return r, err
	}
	if len(p.Plan.SweepDevices) > 0 {
		r.OwnerSweep, err = sweep(ctx, &p, r.EvidenceDir, factory)
		r.Plan, r.Trace = p.Plan, p.Trace
		r.Devices = initialDevices(p.Plan.Devices)
		if err != nil {
			return r, err
		}
	}
	jobs := make(chan int)
	failures := make(chan error, len(p.Plan.Devices))
	var wg sync.WaitGroup
	for range p.Options.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				result, e := runDevice(ctx, p, p.Plan.Devices[i], i, r.EvidenceDir, factory)
				r.Devices[i] = result
				if e != nil {
					failures <- e
					cancel()
				}
			}
		}()
	}
schedule:
	for i := range p.Plan.Devices {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break schedule
		}
	}
	close(jobs)
	wg.Wait()
	close(failures)
	for e := range failures {
		err = errors.Join(err, e)
	}
	r.Findings = append(evaluate(p, r.Devices), comparisons(p, r.Devices)...)
	if err != nil {
		return r, err
	}
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	usable, complete := 0, 0
	sweepComplete := true
	for _, d := range r.OwnerSweep {
		if d.Status != "complete" {
			sweepComplete = false
		}
		for _, c := range d.Commands {
			if c.Status == "ok" {
				usable++
			}
		}
	}
	for _, d := range r.Devices {
		if d.Status == "complete" {
			complete++
		}
		for _, c := range d.Commands {
			if c.Status == "ok" {
				usable++
			}
		}
	}
	if usable == 0 {
		return r, ErrNoResults
	}
	r.Status = "partial"
	if complete == len(r.Devices) && sweepComplete {
		r.Status = "complete"
	}
	return r, nil
}

func runDevice(ctx context.Context, p Prepared, dp DevicePlan, index int, dir string, factory Factory) (r DeviceResult, err error) {
	emit(p.Options, dp.Device.Name, "", "connecting")
	defer func() { emit(p.Options, dp.Device.Name, "", r.Status) }()
	r = DeviceResult{Name: dp.Device.Name, Status: dp.Status, Reason: dp.Reason, Interfaces: []string{}, Commands: []CommandResult{}}
	if dp.Status != "planned" {
		return r, nil
	}
	drv, _ := Registry(dp.Device.Platform)
	params := parameters(p, nil)
	session, e := factory(ctx, dp, func(s string) error { return drv.Validate(s, params) })
	if e != nil {
		r.Status, r.Reason = "failed", e.Error()
		return r, nil
	}
	defer func() {
		if e := session.Close(); e != nil {
			r.Status, r.Reason = "partial", "session close: "+e.Error()
		}
	}()
	commands := dp.Commands
	for phase := 0; phase < 2; phase++ {
		for _, c := range commands {
			if phase == 0 && (c.Group == "counters" || c.Group == "lldp") {
				continue
			}
			if phase == 1 && c.Group != "counters" && c.Group != "lldp" {
				continue
			}
			cr := CommandResult{PlannedCommand: c}
			if c.Status == "planned" && ctx.Err() != nil {
				cr.Status, cr.Reason = "cancelled", ctx.Err().Error()
			} else if c.Status == "planned" {
				emit(p.Options, dp.Device.Name, c.Name, "running")
				cr, err = execute(ctx, drv, session, c, index, len(r.Commands), dir)
				emit(p.Options, dp.Device.Name, c.Name, cr.Status)
			}
			if cr.Facts != nil && phase == 1 {
				if c.Group == "counters" {
					cr.Facts.Counters = slices.DeleteFunc(cr.Facts.Counters, func(x driver.Counter) bool { return c.Name != "counters-"+x.Interface })
				}
				if c.Group == "lldp" {
					cr.Facts.Neighbors = slices.DeleteFunc(cr.Facts.Neighbors, func(x driver.Neighbor) bool { return !slices.Contains(r.Interfaces, x.Interface) })
				}
			}
			r.Commands = append(r.Commands, cr)
			if err != nil {
				r.Status = "failed"
				r.Reason = err.Error()
				return r, err
			}
		}
		if phase == 0 {
			r.Interfaces = interfaces(dp.HopAddresses, r.Commands)
			params.Interfaces = r.Interfaces
			plannedCommands, e := drv.Plan(p.Options.Groups, params)
			if e != nil {
				r.Status, r.Reason = "partial", e.Error()
				return r, nil
			}
			commands, e = planned(plannedCommands, dp, p.Options, drv, params)
			if e != nil {
				r.Status, r.Reason = "partial", e.Error()
				return r, nil
			}
		}
	}
	r.Status = "complete"
	for _, c := range r.Commands {
		if c.Status != "ok" {
			r.Status = "partial"
		}
	}
	return r, nil
}
func execute(ctx context.Context, drv driver.Driver, session device.Session, c PlannedCommand, di, ci int, dir string) (CommandResult, error) {
	r := CommandResult{PlannedCommand: c}
	started := time.Now()
	start := stamp()
	r.StartedAt = &start
	result, runErr := session.Run(ctx, c.Text)
	end := stamp()
	r.EndedAt = &end
	elapsed := time.Since(started).Seconds()
	r.ElapsedSeconds = &elapsed
	r.Status, r.Reason, r.ExitCode = result.Status, result.Error, result.ExitCode
	if len(result.Argv) > 0 {
		r.Argv = result.Argv
	}
	if runErr != nil {
		r.Status, r.Reason = "failed", runErr.Error()
	}
	var err error
	r.Stdout, err = evidence.Write(dir, fmt.Sprintf("device-%03d-command-%03d.stdout", di, ci), result.Stdout)
	if err != nil {
		return r, err
	}
	r.Stderr, err = evidence.Write(dir, fmt.Sprintf("device-%03d-command-%03d.stderr", di, ci), result.Stderr)
	if err != nil {
		return r, err
	}
	if r.Status != "ok" || c.Group == "extra" {
		return r, nil
	}
	facts, err := drv.Parse(c.Name, result.Stdout)
	if err != nil {
		r.Status, r.Reason = "parse_failed", err.Error()
		return r, nil
	}
	r.Facts = &facts
	return r, nil
}
func interfaces(hops []string, commands []CommandResult) []string {
	out := []string{}
	for _, c := range commands {
		if c.Facts == nil {
			continue
		}
		for _, a := range c.Facts.Addresses {
			if slices.Contains(hops, a.Address) {
				out = append(out, a.Interface)
			}
		}
		for _, r := range c.Facts.Routes {
			if r.Selected == nil || !*r.Selected || r.Installed == nil || !*r.Installed {
				continue
			}
			for _, n := range r.NextHops {
				if n.Active != nil && *n.Active && n.Installed != nil && *n.Installed && n.Interface != "" {
					out = append(out, n.Interface)
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
func evaluate(p Prepared, devices []DeviceResult) []Finding {
	out := []Finding{}
	for i, d := range devices {
		wanted := p.Plan.Devices[i].HopAddresses
		for _, c := range d.Commands {
			if c.Name != "addresses" || c.Status != "ok" || c.Facts == nil {
				continue
			}
			found := []string{}
			for _, a := range c.Facts.Addresses {
				if !slices.Contains(wanted, a.Address) {
					continue
				}
				found = append(found, a.Address)
				if a.Master != "" {
					out = append(out, finding("interface-context", d.Name, "Matched address "+a.Address+" belongs to "+a.Interface+" with master "+a.Master+"; requested routing context is "+p.Plan.Context+". Do not infer the trace's routing context from this lookup.", []string{c.Stdout.Path + "#" + a.Evidence}))
				}
			}
			for _, ip := range wanted {
				if !slices.Contains(found, ip) {
					out = append(out, finding("owner-not-observed", d.Name, "Matched address "+ip+" was absent from the collected address snapshot; inventory identity or observation timing may differ.", []string{c.Stdout.Path}))
				}
			}
		}
	}
	return out
}
func finding(kind, device, text string, evidence []string) Finding {
	return Finding{Kind: kind, Device: device, Text: text, Evidence: evidence, Rule: "v0.2/" + kind, Severity: "info", Limitations: []string{"Device snapshots and the saved trace have different observation windows; they do not establish the earlier forwarding path or a fault."}}
}

func initialDevices(plans []DevicePlan) []DeviceResult {
	out := make([]DeviceResult, len(plans))
	for i, dp := range plans {
		out[i] = DeviceResult{Name: dp.Device.Name, Status: "cancelled", Reason: "not scheduled", Interfaces: []string{}, Commands: []CommandResult{}}
	}
	return out
}
func cloneMatches(hops []trace.Hop) []trace.Hop {
	out := slices.Clone(hops)
	for i := range out {
		out[i].Matches = maps.Clone(out[i].Matches)
	}
	return out
}
