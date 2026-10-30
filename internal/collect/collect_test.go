package collect

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/device"
	"github.com/amb1s1/tracefull/internal/process"
	"github.com/amb1s1/tracefull/internal/schema"
	"github.com/amb1s1/tracefull/internal/trace"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func capture(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "driver", "cumulus", "testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeSession struct {
	t               *testing.T
	policy          device.Policy
	commands        []string
	fail, malformed string
	block           bool
	closed          bool
}

func (s *fakeSession) Run(ctx context.Context, cmd string) (device.Result, error) {
	if err := s.policy(cmd); err != nil {
		return device.Result{}, err
	}
	s.commands = append(s.commands, cmd)
	if s.block {
		<-ctx.Done()
		return device.Result{Result: process.Result{Status: "cancelled", Error: ctx.Err().Error()}}, nil
	}
	code := 0
	r := device.Result{Result: process.Result{Status: "ok", ExitCode: &code}, Command: cmd}
	if cmd == s.fail {
		code = 1
		r.Status = "failed"
		r.Stderr = []byte("permission denied")
		return r, nil
	}
	if cmd == s.malformed {
		r.Stdout = []byte("invalid JSON")
		return r, nil
	}
	var file string
	switch {
	case cmd == "ip -j address show":
		file = "addresses.json"
	case cmd == "vtysh -c 'show version'" || cmd == "sudo -n vtysh -c 'show version'":
		file = "frr-version.txt"
	case strings.Contains(cmd, "vtysh"):
		file = "frr-route.json"
	case strings.Contains(cmd, "ip -j -s link"):
		file = "counters.json"
	case strings.Contains(cmd, "lldpctl"):
		file = "lldp.json"
	case cmd == "cat /etc/os-release":
		file = "os-release.txt"
	case cmd == "cat /proc/uptime":
		file = "uptime.txt"
	case cmd == "cat /proc/loadavg":
		file = "loadavg.txt"
	default:
		s.t.Fatalf("unexpected command %q", cmd)
	}
	r.Stdout = capture(s.t, file)
	return r, nil
}
func (s *fakeSession) Close() error { s.closed = true; return nil }
func factory(s *fakeSession) Factory {
	return func(_ context.Context, _ DevicePlan, p device.Policy) (device.Session, error) {
		s.policy = p
		return s, nil
	}
}

func TestCapturePipeline(t *testing.T) {
	p := prepare(t, nil)
	if len(p.Plan.Devices) != 1 || p.Plan.Devices[0].Transport.User != "operator" {
		t.Fatal(p.Plan)
	}
	for _, c := range p.Plan.Devices[0].Commands {
		if c.Group == "counters" || c.Group == "lldp" {
			if c.Status != "deferred" || c.Text != "" {
				t.Fatal(c)
			}
		}
	}
	if _, err := os.Stat(p.Options.EvidenceDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run wrote evidence")
	}
	s := &fakeSession{t: t}
	r, err := Run(context.Background(), p, factory(s))
	if err != nil {
		t.Fatal(err)
	}
	if !s.closed || r.Status != "complete" {
		t.Fatal(r.Status, s.closed)
	}
	if len(r.Devices[0].Interfaces) != 9 || !slices.Contains(r.Devices[0].Interfaces, "eth0") {
		t.Fatal(r.Devices[0].Interfaces)
	}
	if len(r.Findings) != 1 || r.Findings[0].Kind != "interface-context" {
		t.Fatal(r.Findings)
	}
	seenRoute := false
	counterCount := 0
	for _, c := range r.Devices[0].Commands {
		if c.Name == "route" {
			seenRoute = true
			if len(c.Facts.Routes[0].NextHops) != 8 {
				t.Fatal(c.Facts)
			}
		}
		if c.Group == "counters" {
			counterCount++
			if !seenRoute || len(c.Facts.Counters) != 1 {
				t.Fatal(c)
			}
		}
		if c.Stdout == nil || c.Stderr == nil || c.StartedAt == nil || c.ElapsedSeconds == nil {
			t.Fatal(c)
		}
		for _, ev := range []string{c.Stdout.Path, c.Stderr.Path} {
			info, e := os.Stat(filepath.Join(r.EvidenceDir, ev))
			if e != nil || info.Mode().Perm() != 0600 {
				t.Fatal(info, e)
			}
		}
		data, e := os.ReadFile(filepath.Join(r.EvidenceDir, c.Stdout.Path))
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != c.Stdout.SHA256 || len(data) != c.Stdout.Bytes {
			t.Fatal("evidence digest", e)
		}
	}
	if counterCount != 9 {
		t.Fatal(counterCount)
	}
	info, err := os.Stat(r.EvidenceDir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal(info, err)
	}
	validateReport(t, r)
	for _, format := range []string{"json", "text", "markdown"} {
		var out bytes.Buffer
		if err := Render(&out, r, format); err != nil {
			t.Fatal(err)
		}
		if format == "json" && !json.Valid(out.Bytes()) {
			t.Fatal("JSON")
		}
		if err := RenderPlan(&out, p.Plan, format); err != nil {
			t.Fatal(err)
		}
	}
}
func validateReport(t *testing.T, r Report) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for name, data := range map[string][]byte{"report-v1.json": schema.Report, "collection-v1.json": schema.Collection} {
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("https://github.com/amb1s1/tracefull/schema/"+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	s, err := compiler.Compile("https://github.com/amb1s1/tracefull/schema/collection-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(r.EvidenceDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err = s.Validate(doc); err != nil {
		t.Fatal(err)
	}
	doc.(map[string]any)["unexpected"] = true
	if s.Validate(doc) == nil {
		t.Fatal("schema accepts unknown field")
	}
}
func TestPartialAndNoResults(t *testing.T) {
	for _, mode := range []string{"command", "parse", "connect", "unsupported", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			p := prepare(t, nil)
			s := &fakeSession{t: t}
			f := factory(s)
			switch mode {
			case "command":
				s.fail = "ip -j address show"
			case "parse":
				s.malformed = "ip -j address show"
			case "connect":
				f = func(context.Context, DevicePlan, device.Policy) (device.Session, error) {
					return nil, errors.New("denied")
				}
			case "unsupported":
				p.Plan.Devices[0].Status = "unsupported"
			case "cancel":
				s.block = true
				p.Options.Timeout = 10 * time.Millisecond
			}
			r, err := Run(context.Background(), p, f)
			if mode == "command" || mode == "parse" {
				if err != nil || r.Status != "partial" {
					t.Fatal(r.Status, err)
				}
				if len(r.Findings) != 0 {
					t.Fatal("failed address read asserted ownership", r.Findings)
				}
			} else if mode == "cancel" {
				if !errors.Is(err, context.DeadlineExceeded) || r.Status != "interrupted" || !s.closed {
					t.Fatal(r.Status, err)
				}
			} else if !errors.Is(err, ErrNoResults) {
				t.Fatal(err)
			}
			validateReport(t, r)
		})
	}
}
func TestEvidenceWriteFailure(t *testing.T) {
	p := prepare(t, nil)
	s := &fakeSession{t: t}
	f := func(ctx context.Context, dp DevicePlan, policy device.Policy) (device.Session, error) {
		// A conflicting file deterministically injects failure even under privileged tests.
		if err := os.WriteFile(filepath.Join(p.Options.EvidenceDir, "device-000-command-000.stdout"), []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		return factory(s)(ctx, dp, policy)
	}
	r, err := Run(context.Background(), p, f)
	if err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatal(err)
	}
	if !s.closed {
		t.Fatal("session leaked")
	}
	data, e := os.ReadFile(filepath.Join(r.EvidenceDir, "device-000-command-000.stdout"))
	if e != nil || string(data) != "preserve" {
		t.Fatal("overwritten", e)
	}
	if _, e = os.Stat(filepath.Join(r.EvidenceDir, "report.json")); e != nil {
		t.Fatal(e)
	}
	if _, err = Run(context.Background(), p, f); err == nil {
		t.Fatal("reused evidence directory")
	}
}
func TestBoundedDeviceConcurrency(t *testing.T) {
	p := prepare(t, nil)
	base := p.Plan.Devices[0]
	for i := 1; i < 8; i++ {
		dp := base
		dp.Device.Name = fmt.Sprint(i)
		p.Plan.Devices = append(p.Plan.Devices, dp)
	}
	p.Options.Workers = 2
	var active, max atomic.Int32
	f := func(ctx context.Context, _ DevicePlan, _ device.Policy) (device.Session, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-time.After(5 * time.Millisecond):
		case <-ctx.Done():
		}
		return nil, errors.New("test failure")
	}
	_, err := Run(context.Background(), p, f)
	if !errors.Is(err, ErrNoResults) || max.Load() > 2 || max.Load() < 1 || active.Load() != 0 {
		t.Fatal(err, max.Load(), active.Load())
	}
}

func TestOwnerSweep(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprint(ambiguous), func(t *testing.T) {
			p := prepare(t, nil)
			// Synthetic inventory changes: the real capture address is absent from the
			// initial inventory identity. The address snapshot resolves it, not a prefix.
			p.Trace.Hops[0].Matches["2001:db8:2::3"] = trace.Match{Method: "unmatched", Candidates: []string{}}
			dp := p.Plan.Devices[0]
			dp.HopAddresses = []string{}
			p.Plan.Devices = []DevicePlan{}
			p.Plan.SweepDevices = []DevicePlan{dp}
			p.Options.OwnerSweep = true
			// Use the same address-only planner as Prepare.
			var err error
			p.Plan.SweepDevices[0], err = devicePlan(p, dp.Device, []string{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if ambiguous {
				other := p.Plan.SweepDevices[0]
				other.Device.Name = "switch-b"
				p.Plan.SweepDevices = append(p.Plan.SweepDevices, other)
			}
			var calls atomic.Int32
			f := func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
				calls.Add(1)
				return &fakeSession{t: t, policy: policy}, nil
			}
			r, err := Run(context.Background(), p, f)
			if err != nil {
				t.Fatal(err)
			}
			m := r.Trace.Hops[0].Matches["2001:db8:2::3"]
			if ambiguous {
				if m.Device != "" || m.Method != "owner-sweep-ambiguous" || len(r.Devices) != 0 || calls.Load() != 2 {
					t.Fatal(m, calls.Load())
				}
			} else {
				if m.Device != "switch-a" || m.Method != "owner-sweep" || len(r.Devices) != 1 || calls.Load() != 2 {
					t.Fatal(m, calls.Load())
				}
			}
			for _, d := range r.OwnerSweep {
				if len(d.Commands) != 1 || d.Commands[0].Name != "addresses" {
					t.Fatal(d)
				}
			}
			validateReport(t, r)
		})
	}
}
