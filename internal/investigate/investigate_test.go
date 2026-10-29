package investigate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type runnerFunc func(context.Context, Command) ProcessResult

func (f runnerFunc) Run(ctx context.Context, c Command) ProcessResult { return f(ctx, c) }
func fakeLookup(s string) (string, error)                             { return "/fake/" + s, nil }
func testOptions(t *testing.T) Options {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	o := Defaults()
	o.Target = "192.0.2.2"
	o.EvidenceDir = filepath.Join(t.TempDir(), "evidence")
	return o
}

// These are synthetic tool outputs; no executable sends diagnostic packets.
func successful(_ context.Context, c Command) ProcessResult {
	code := 0
	r := ProcessResult{ExitCode: &code, Status: "ok"}
	switch c.Tool {
	case "traceroute":
		r.Stderr = []byte("traceroute to dst (192.0.2.2), 30 hops max\n")
		r.Stdout = []byte("1 192.0.2.1 1 ms\n2 192.0.2.2 2 ms\n")
	case "mtr":
		r.Stdout = []byte(`{"report":{"mtr":{"dst":"192.0.2.2","tests":100},"hubs":[{"count":1,"host":"192.0.2.2","Snt":100,"Loss%":0,"Best":1,"Avg":2,"Wrst":3}]}}`)
	case "tracepath":
		r.Stdout = []byte("1: 192.0.2.2 1ms pmtu 1500 reached\nResume: pmtu 1500 hops 1 back 1\n")
	}
	return r
}
func deps() Dependencies {
	return Dependencies{GOOS: "linux", Lookup: fakeLookup, Runner: runnerFunc(successful)}
}

func TestPlanSafetyAndPlatforms(t *testing.T) {
	o := testOptions(t)
	for _, target := range []string{"--help", "bad;host", "a b", "$(id)", "a/b", "", "fe80::1%en0"} {
		o.Target = target
		if _, err := BuildPlan(o, "linux", fakeLookup); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	o.Target = "host.example"
	p, err := BuildPlan(o, "linux", func(string) (string, error) {
		t.Fatal("hostname dry-run must defer executable selection")
		return "", nil
	})
	if err != nil || !p.ResolutionRequired {
		t.Fatal(p, err)
	}
	for _, c := range p.Commands {
		if c.Status != "deferred" || len(c.Args) != 0 {
			t.Fatal(c)
		}
	}
	o.Target = "2001:db8::1"
	p, err = BuildPlan(o, "darwin", fakeLookup)
	if err != nil || p.Commands[0].Executable != "/fake/traceroute6" || p.Commands[2].Status != "unsupported" {
		t.Fatal(p, err)
	}
	p, err = BuildPlan(o, "linux", fakeLookup)
	if err != nil || p.Commands[0].Args[0] != "-6" {
		t.Fatal(p, err)
	}
	o.Tools = []string{"traceroute", "traceroute"}
	if Validate(o) == nil {
		t.Fatal("duplicate tool accepted")
	}
	o.Tools = []string{"sh"}
	if Validate(o) == nil {
		t.Fatal("arbitrary tool accepted")
	}
	o.Tools = nil
	o.Cycles = 0
	if Validate(o) == nil {
		t.Fatal("zero cycles")
	}
	o.Cycles = 100
	o.ToolTimeout = o.Timeout + time.Second
	if Validate(o) == nil {
		t.Fatal("unbounded tool")
	}
}

func TestPipelineAndSchema(t *testing.T) {
	o := testOptions(t)
	r, err := Run(context.Background(), o, deps())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "complete" || len(r.Results) != 3 {
		t.Fatal(r)
	}
	for _, result := range r.Results {
		if result.Report == nil || result.StartedAt == nil || result.EndedAt == nil {
			t.Fatal(result)
		}
		for _, ev := range []*Evidence{result.Stdout, result.Stderr, result.Input} {
			data, err := os.ReadFile(filepath.Join(r.EvidenceDir, ev.Path))
			if err != nil {
				t.Fatal(err)
			}
			if len(data) != ev.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != ev.SHA256 {
				t.Fatal("invalid evidence digest")
			}
			info, _ := os.Stat(filepath.Join(r.EvidenceDir, ev.Path))
			if info.Mode().Perm() != 0600 {
				t.Fatal(info.Mode())
			}
		}
	}
	info, _ := os.Stat(r.EvidenceDir)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
	compiler := jsonschema.NewCompiler()
	for name, data := range map[string][]byte{"report-v1.json": schema.Report, "investigation-v1.json": schema.Investigation} {
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("https://tracefull.test/"+name, doc); err != nil {
			t.Fatal(err)
		}
	}
	s, err := compiler.Compile("https://tracefull.test/investigation-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(r.EvidenceDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(doc); err != nil {
		t.Fatal(err)
	}
	doc.(map[string]any)["unexpected"] = true
	if s.Validate(doc) == nil {
		t.Fatal("schema accepted unknown field")
	}
	for _, format := range []string{"json", "text", "markdown"} {
		var out bytes.Buffer
		if err := Render(&out, r, format); err != nil {
			t.Fatal(err)
		}
		if format == "json" && !json.Valid(out.Bytes()) {
			t.Fatal("invalid JSON")
		}
		if format != "json" && !strings.Contains(out.String(), "samples are not combined") {
			t.Fatal("missing interpretation limit")
		}
	}
	if _, err := Run(context.Background(), o, deps()); err == nil {
		t.Fatal("existing evidence directory overwritten")
	}
}

func TestDefaultEvidenceDirectory(t *testing.T) {
	o := testOptions(t)
	t.Chdir(t.TempDir())
	o.EvidenceDir = ""
	r, err := Run(context.Background(), o, deps())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(r.EvidenceDir), ".tracefull-evidence-") {
		t.Fatalf("evidence must be excluded from Go package discovery: %s", r.EvidenceDir)
	}
	if _, err := os.Stat(filepath.Join(r.EvidenceDir, "report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFailuresAndCancellation(t *testing.T) {
	for _, status := range []string{"failed", "timeout", "truncated", "cancelled", "parse_failed"} {
		t.Run(status, func(t *testing.T) {
			o := testOptions(t)
			d := deps()
			d.Runner = runnerFunc(func(ctx context.Context, c Command) ProcessResult {
				if c.Tool == "mtr" {
					if status == "parse_failed" {
						return ProcessResult{Status: "ok", Stdout: []byte("bad output")}
					}
					return ProcessResult{Status: status, Error: "test failure", Stderr: []byte("reason")}
				}
				return successful(ctx, c)
			})
			r, err := Run(context.Background(), o, d)
			if err != nil || r.Status != "partial" || r.Results[1].Status != status || r.Results[1].Report != nil {
				t.Fatal(r, err)
			}
		})
	}
	t.Run("all missing", func(t *testing.T) {
		o := testOptions(t)
		d := deps()
		d.Lookup = func(string) (string, error) { return "", os.ErrNotExist }
		r, err := Run(context.Background(), o, d)
		if !errors.Is(err, ErrNoResults) || r.Status != "failed" {
			t.Fatal(r, err)
		}
		if _, err := os.Stat(filepath.Join(o.EvidenceDir, "report.json")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancel preserves evidence", func(t *testing.T) {
		o := testOptions(t)
		d := deps()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		d.Runner = runnerFunc(func(context.Context, Command) ProcessResult {
			calls++
			cancel()
			return ProcessResult{Status: "cancelled", Stdout: []byte("partial")}
		})
		r, err := Run(ctx, o, d)
		if !errors.Is(err, context.Canceled) || calls != 1 || r.Status != "interrupted" {
			t.Fatal(r, err, calls)
		}
		if _, err := os.Stat(filepath.Join(o.EvidenceDir, "report.json")); err != nil {
			t.Fatal(err)
		}
	})
}

func TestResolutionAndWriteFailures(t *testing.T) {
	o := testOptions(t)
	o.Target = "host.example"
	d := deps()
	lookups := 0
	d.Resolve = func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("192.0.2.2")}, nil
	}
	d.Runner = runnerFunc(func(ctx context.Context, c Command) ProcessResult {
		if c.Args[len(c.Args)-1] != "192.0.2.2" {
			t.Fatal(c)
		}
		return successful(ctx, c)
	})
	r, err := Run(context.Background(), o, d)
	if err != nil || lookups != 1 || r.Plan.Target != "host.example" || r.Plan.Address != "192.0.2.2" {
		t.Fatal(r, err)
	}
	t.Run("raw write failure", func(t *testing.T) {
		o := testOptions(t)
		d := deps()
		d.Runner = runnerFunc(func(ctx context.Context, c Command) ProcessResult {
			if err := os.Mkdir(filepath.Join(o.EvidenceDir, c.Tool+".stdout"), 0700); err != nil {
				t.Fatal(err)
			}
			return successful(ctx, c)
		})
		r, err := Run(context.Background(), o, d)
		if err == nil || r.Status != "failed" {
			t.Fatal(r, err)
		}
		if _, err := os.Stat(filepath.Join(o.EvidenceDir, "report.json")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("final report failure", func(t *testing.T) {
		o := testOptions(t)
		o.Tools = []string{"traceroute"}
		d := deps()
		d.Runner = runnerFunc(func(ctx context.Context, c Command) ProcessResult {
			if err := os.WriteFile(filepath.Join(o.EvidenceDir, "report.json"), []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			return successful(ctx, c)
		})
		_, err := Run(context.Background(), o, d)
		if err == nil || !strings.Contains(err.Error(), "finalize report") {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(o.EvidenceDir, "report.json"))
		if !reflect.DeepEqual(data, []byte("existing")) {
			t.Fatal("overwritten report")
		}
	})
}
