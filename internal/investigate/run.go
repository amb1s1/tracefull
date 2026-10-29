package investigate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/amb1s1/tracefull/internal/app"
	"github.com/amb1s1/tracefull/internal/config"
	"github.com/amb1s1/tracefull/internal/report"
	"github.com/amb1s1/tracefull/internal/verdict"
)

var ErrNoResults = errors.New("no diagnostic tool produced a usable report")

type Evidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}
type Result struct {
	Command
	StartedAt *string        `json:"started_at"`
	EndedAt   *string        `json:"ended_at"`
	ExitCode  *int           `json:"exit_code"`
	Stdout    *Evidence      `json:"stdout"`
	Stderr    *Evidence      `json:"stderr"`
	Input     *Evidence      `json:"input"`
	Report    *report.Report `json:"report"`
}
type Report struct {
	SchemaVersion string   `json:"schema_version"`
	Kind          string   `json:"kind"`
	Plan          Plan     `json:"plan"`
	EvidenceDir   string   `json:"evidence_dir"`
	StartedAt     string   `json:"started_at"`
	EndedAt       string   `json:"ended_at"`
	Status        string   `json:"status"`
	Error         string   `json:"error"`
	Results       []Result `json:"results"`
}
type Dependencies struct {
	GOOS    string
	Lookup  func(string) (string, error)
	Resolve func(context.Context, string) ([]netip.Addr, error)
	Runner  Runner
}

func System() Dependencies {
	return Dependencies{GOOS: runtime.GOOS, Lookup: exec.LookPath, Runner: ProcessRunner{}, Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}}
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Run resolves once, executes sequentially and finalizes a report even after a
// tool fails or the caller cancels. Evidence-write failures remain fatal.
func Run(ctx context.Context, o Options, d Dependencies) (r Report, err error) {
	r = Report{SchemaVersion: "1", Kind: "investigation", StartedAt: stamp(), Results: []Result{}}
	r.Plan, err = BuildPlan(o, d.GOOS, d.Lookup)
	if err != nil {
		return r, err
	}
	if err = app.ValidateInventory(app.Options{Inventory: o.Inventory, UserConfigDir: config.UserDir(), ProjectDir: "."}); err != nil {
		return r, err
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	r.EvidenceDir, err = newDirectory(o.EvidenceDir)
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
		_, writeErr := writeJSON(r.EvidenceDir, "report.json", r)
		if writeErr != nil {
			err = fmt.Errorf("finalize report: %w (execution error: %v)", writeErr, err)
		}
	}()
	if r.Plan.ResolutionRequired {
		address, resolveErr := resolve(ctx, o.Target, d.Resolve)
		if resolveErr != nil {
			return r, fmt.Errorf("%w: %v", ErrNoResults, resolveErr)
		}
		resolved := o
		resolved.Target = address
		r.Plan, err = BuildPlan(resolved, d.GOOS, d.Lookup)
		r.Plan.Target = o.Target
		if err != nil {
			return r, err
		}
	}
	usable := 0
	for _, command := range r.Plan.Commands {
		result := Result{Command: command}
		if command.Status == "planned" {
			if ctx.Err() != nil {
				result.Status, result.Reason = "cancelled", ctx.Err().Error()
			} else {
				result, err = execute(ctx, o, d.Runner, command, r.Plan.Address, r.EvidenceDir)
			}
		}
		r.Results = append(r.Results, result)
		if err != nil {
			return r, err
		}
		if result.Status == "ok" {
			usable++
		}
	}
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if usable == 0 {
		return r, ErrNoResults
	}
	r.Status = "complete"
	if usable < len(r.Results) {
		r.Status = "partial"
	}
	return r, nil
}

func resolve(ctx context.Context, target string, lookup func(context.Context, string) ([]netip.Addr, error)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addresses, err := lookup(ctx, target)
	if err != nil {
		return "", fmt.Errorf("resolve target: %w", err)
	}
	valid := []netip.Addr{}
	for _, a := range addresses {
		if a.IsValid() && a.Zone() == "" {
			valid = append(valid, a.Unmap())
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].Is4() != valid[j].Is4() {
			return valid[i].Is4()
		}
		return valid[i].Less(valid[j])
	})
	if len(valid) == 0 {
		return "", fmt.Errorf("target has no usable addresses")
	}
	return valid[0].String(), nil
}

func execute(ctx context.Context, o Options, runner Runner, c Command, address, dir string) (Result, error) {
	r := Result{Command: c}
	start := stamp()
	r.StartedAt = &start
	child, cancel := context.WithTimeout(ctx, o.ToolTimeout)
	p := runner.Run(child, c)
	cancel()
	end := stamp()
	r.EndedAt = &end
	r.ExitCode = p.ExitCode
	r.Status = p.Status
	r.Reason = p.Error
	var err error
	r.Stdout, err = writeEvidence(dir, c.Tool+".stdout", p.Stdout)
	if err != nil {
		return r, err
	}
	r.Stderr, err = writeEvidence(dir, c.Tool+".stderr", p.Stderr)
	if err != nil {
		return r, err
	}
	if p.Status != "ok" {
		return r, nil
	}
	input := p.Stdout
	if c.Tool == "traceroute" {
		input = append(append(append([]byte{}, p.Stderr...), '\n'), p.Stdout...)
	}
	r.Input, err = writeEvidence(dir, c.Tool+".input", input)
	if err != nil {
		return r, err
	}
	parsed, parseErr := app.Build(ctx, app.Options{From: filepath.Join(dir, r.Input.Path), Dest: address, Inventory: o.Inventory, UserConfigDir: config.UserDir(), ProjectDir: ".", RDNS: o.RDNS, ASN: o.ASN, Timeout: time.Second, LatencyJump: verdict.DefaultLatencyJump}, nil, net.DefaultResolver)
	if parseErr != nil {
		r.Status = "parse_failed"
		r.Reason = parseErr.Error()
		return r, nil
	}
	if parsed.Probe.Tool != c.Tool {
		r.Status = "parse_failed"
		r.Reason = "output does not match selected tool"
		return r, nil
	}
	// These probe modes follow the adapter's fixed argv, independently of which
	// metadata the tool happens to include in its output.
	parsed.Probe.Type = "udp"
	if c.Tool == "mtr" {
		parsed.Probe.Type = "icmp"
	}
	r.Report = &parsed
	return r, nil
}

func newDirectory(path string) (string, error) {
	var err error
	if path == "" {
		path, err = os.MkdirTemp(".", "tracefull-evidence-")
	} else {
		err = os.Mkdir(path, 0700)
	}
	if err != nil {
		return "", fmt.Errorf("create new evidence directory: %w", err)
	}
	return filepath.Abs(path)
}
func writeEvidence(dir, name string, data []byte) (*Evidence, error) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create evidence: %w", err)
	}
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return nil, fmt.Errorf("persist evidence: %w", err)
	}
	return &Evidence{Path: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: len(data)}, nil
}
func writeJSON(dir, name string, value any) (*Evidence, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return writeEvidence(dir, name, append(data, '\n'))
}
