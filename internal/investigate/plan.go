// Package investigate orchestrates bounded local tools without a shell.
package investigate

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxOutputBytes = 1 << 20

type Options struct {
	Target, EvidenceDir, Inventory string
	Tools                          []string
	MaxHops, Cycles                int
	Timeout, ToolTimeout           time.Duration
	RDNS, ASN                      bool
}

func Defaults() Options {
	return Options{MaxHops: 30, Cycles: 100, Timeout: 5 * time.Minute, ToolTimeout: 2 * time.Minute}
}

type Command struct {
	Tool       string   `json:"tool"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
}

type Plan struct {
	Target             string    `json:"target"`
	Address            string    `json:"address"`
	ResolutionRequired bool      `json:"resolution_required"`
	TimeoutSeconds     float64   `json:"timeout_seconds"`
	ToolTimeoutSeconds float64   `json:"tool_timeout_seconds"`
	MaxOutputBytes     int       `json:"max_output_bytes_per_stream"`
	Commands           []Command `json:"commands"`
}

var hostnameLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func Validate(o Options) error {
	if a, err := netip.ParseAddr(o.Target); err == nil {
		if a.Zone() != "" {
			return fmt.Errorf("scoped addresses are not supported")
		}
	} else {
		if len(o.Target) == 0 || len(o.Target) > 253 {
			return fmt.Errorf("target must be an IP address or hostname")
		}
		for _, label := range strings.Split(strings.TrimSuffix(o.Target, "."), ".") {
			if !hostnameLabel.MatchString(label) {
				return fmt.Errorf("target must be an IP address or hostname")
			}
		}
	}
	if o.MaxHops < 1 || o.MaxHops > 255 || o.Cycles < 1 || o.Cycles > 1000 {
		return fmt.Errorf("max-hops must be 1–255 and cycles 1–1000")
	}
	if o.Timeout <= 0 || o.Timeout > 30*time.Minute || o.ToolTimeout <= 0 || o.ToolTimeout > o.Timeout {
		return fmt.Errorf("timeouts must be positive, tool-timeout <= timeout, and timeout <= 30m")
	}
	seen := map[string]bool{}
	for _, name := range selected(o.Tools) {
		if name != "traceroute" && name != "mtr" && name != "tracepath" {
			return fmt.Errorf("unknown tool %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate tool %q", name)
		}
		seen[name] = true
	}
	return nil
}

func selected(names []string) []string {
	if len(names) == 0 {
		return []string{"traceroute", "mtr", "tracepath"}
	}
	return names
}

// BuildPlan is also used for execution after resolving the destination once.
// A hostname dry-run intentionally has no executable argv until resolution.
func BuildPlan(o Options, goos string, lookup func(string) (string, error)) (Plan, error) {
	p := Plan{Target: o.Target, TimeoutSeconds: o.Timeout.Seconds(), ToolTimeoutSeconds: o.ToolTimeout.Seconds(), MaxOutputBytes: MaxOutputBytes, Commands: []Command{}}
	if err := Validate(o); err != nil {
		return p, err
	}
	if goos != "linux" && goos != "darwin" {
		return p, fmt.Errorf("investigation requires Linux or macOS")
	}
	a, err := netip.ParseAddr(o.Target)
	p.ResolutionRequired = err != nil
	if err == nil {
		a = a.Unmap()
		p.Address = a.String()
	}
	for _, name := range selected(o.Tools) {
		c := Command{Tool: name, Args: []string{}, Status: "planned"}
		if name == "tracepath" && goos != "linux" {
			c.Status, c.Reason = "unsupported", "tracepath execution is supported on Linux only"
		} else if p.ResolutionRequired {
			c.Status, c.Reason = "deferred", "executable and argv require destination resolution; dry-run performs no DNS"
		} else {
			binary, args := commandArgs(name, goos, a, o)
			c.Args = args
			path, err := lookup(binary)
			if err != nil {
				c.Status, c.Reason = "unavailable", binary+" not found in PATH"
			} else {
				c.Executable, err = filepath.Abs(path)
				if err != nil {
					return p, err
				}
			}
		}
		p.Commands = append(p.Commands, c)
	}
	return p, nil
}

// Tool adapters are registered explicitly here. Arguments are never shell text.
func commandArgs(name, goos string, addr netip.Addr, o Options) (string, []string) {
	family := "-4"
	if addr.Is6() {
		family = "-6"
	}
	maxHops := strconv.Itoa(o.MaxHops)
	switch name {
	case "mtr":
		return name, []string{family, "--report", "--json", "--no-dns", "--report-cycles", strconv.Itoa(o.Cycles), "--max-ttl", maxHops, addr.String()}
	case "tracepath":
		return name, []string{family, "-n", "-m", maxHops, addr.String()}
	default:
		args := []string{"-n", "-m", maxHops, "-q", "3", "-w", "1"}
		if goos == "linux" {
			args = append([]string{family}, args...)
		} else if addr.Is6() {
			name = "traceroute6"
		}
		return name, append(args, addr.String())
	}
}
