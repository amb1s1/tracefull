package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/schema"
	"github.com/amb1s1/tracefull/internal/trace"
	"github.com/amb1s1/tracefull/internal/verdict"
)

func sample() Report {
	h := trace.NewHop(1, "line:1")
	h.AddAddress("192.0.2.1")
	h.Names = []string{"evil\x1b[31m|<script>"}
	h.Samples([]float64{2}, 0)
	h.Notes = []string{"untrusted [link](https://evil.example)"}
	return Report{SchemaVersion: "1", Version: "test", Trace: trace.Trace{Dest: "dst", DestAddr: "192.0.2.1", Reached: trace.Ptr(true), Notes: []string{}, Hops: []trace.Hop{h}, Paths: []trace.Path{}}, Source: Source{Name: "synthetic", SHA256: strings.Repeat("a", 64), Bytes: 1}, Findings: []verdict.Finding{}, Devices: []inventory.Device{}}
}
func TestSchemaAndRender(t *testing.T) {
	var schemaDoc any
	if e := json.Unmarshal(schema.Report, &schemaDoc); e != nil {
		t.Fatal(e)
	}
	c := jsonschema.NewCompiler()
	if e := c.AddResource("report.json", schemaDoc); e != nil {
		t.Fatal(e)
	}
	s, e := c.Compile("report.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, format := range []string{"json", "markdown", "text"} {
		var b bytes.Buffer
		r := sample()
		before, _ := json.Marshal(r)
		if e := Render(&b, r, format); e != nil {
			t.Fatal(e)
		}
		after, _ := json.Marshal(r)
		if !bytes.Equal(before, after) {
			t.Fatal("renderer mutated report")
		}
		if strings.Contains(b.String(), "\x1b") {
			t.Fatal("ANSI injection")
		}
		if format == "json" {
			var doc any
			if e := json.Unmarshal(b.Bytes(), &doc); e != nil {
				t.Fatal(e)
			}
			if e := s.Validate(doc); e != nil {
				t.Fatal(e)
			}
			doc.(map[string]any)["unexpected"] = true
			if s.Validate(doc) == nil {
				t.Fatal("schema accepted unknown field")
			}
		}
		if format == "markdown" && (strings.Contains(b.String(), "<script>") || strings.Contains(b.String(), "[link]")) {
			t.Fatal("Markdown injection")
		}
	}
}

type broken struct{}

func (broken) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestWriteFailure(t *testing.T) {
	for _, f := range []string{"json", "text", "markdown", "invalid"} {
		if Render(broken{}, sample(), f) == nil {
			t.Fatal("write error hidden")
		}
	}
}

// Goldens are synthetic expected renderings, never represented as captures.
func TestGolden(t *testing.T) {
	for _, format := range []string{"json", "text", "markdown"} {
		t.Run(format, func(t *testing.T) {
			var b bytes.Buffer
			if e := Render(&b, sample(), format); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join("testdata", "report."+format)
			if os.Getenv("TRACEFULL_UPDATE_GOLDENS") == "1" {
				if e := os.WriteFile(path, b.Bytes(), 0600); e != nil {
					t.Fatal(e)
				}
			}
			want, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(b.Bytes(), want) {
				t.Fatalf("render differs from %s; review before explicitly updating", path)
			}
		})
	}
}

func TestDisplayPrecisionPreservesJSON(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{{11.199999999999998, "11.2"}, {66.66666666666667, "66.667"}, {17.822666666666667, "17.823"}, {10.103, "10.103"}, {0, "0"}, {-0.0001, "0"}} {
		if got := Number(&tc.value); got != tc.want {
			t.Errorf("%v: got %s want %s", tc.value, got, tc.want)
		}
	}
	r := sample()
	r.Hops[0].Avg = trace.Ptr(11.199999999999998)
	for _, format := range []string{"text", "markdown"} {
		var b bytes.Buffer
		if e := Render(&b, r, format); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(b.String(), "11.199999999999998") || !strings.Contains(b.String(), "11.2") {
			t.Fatal("unrounded display")
		}
	}
	var b bytes.Buffer
	if e := Render(&b, r, "json"); e != nil {
		t.Fatal(e)
	}
	var got Report
	if e := json.Unmarshal(b.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if *got.Hops[0].Avg != *r.Hops[0].Avg {
		t.Fatal("display rounding changed JSON measurement")
	}
}

func TestMultipleResponderLabelPreservesJSON(t *testing.T) {
	r := sample()
	r.Hops[0].AddAddress("192.0.2.2")
	r.Findings = verdict.Evaluate(r.Trace, 0)
	for _, format := range []string{"text", "markdown"} {
		var b bytes.Buffer
		if e := Render(&b, r, format); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(b.String(), "warn multiple responders / paths at hop 1") || strings.Contains(b.String(), "warn ecmp at") {
			t.Fatalf("misleading display label: %s", b.String())
		}
	}
	var b bytes.Buffer
	if e := Render(&b, r, "json"); e != nil {
		t.Fatal(e)
	}
	var got Report
	if e := json.Unmarshal(b.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if len(got.Findings) != 1 || got.Findings[0].Kind != "ecmp" || got.Findings[0].Rule != "v0.1/ecmp" || got.Findings[0].Severity != "warn" {
		t.Fatalf("JSON compatibility changed: %+v", got.Findings)
	}
}

func TestCompactReportPreservesAttribution(t *testing.T) {
	r := sample()
	h := &r.Hops[0]
	h.Names = []string{"192.0.2.1", "router.example", "router.example"}
	h.AddAddress("192.0.2.2")
	h.Matches = map[string]trace.Match{
		"192.0.2.1": {Method: "unmatched", Candidates: []string{}},
		"192.0.2.2": {Method: "unmatched", Candidates: []string{}},
	}
	r.Findings = []verdict.Finding{
		{Kind: "ecmp", Hop: 1, Limitations: []string{"shared", "first two only"}},
		{Kind: "jitter", Hop: 2, Limitations: []string{"shared", "first two only"}},
		{Kind: "nat", Hop: 3, Limitations: []string{"shared", "third only"}},
	}
	before, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	for _, format := range []string{"text", "markdown"} {
		var b bytes.Buffer
		if e := Render(&b, r, format); e != nil {
			t.Fatal(e)
		}
		out := b.String()
		if !strings.Contains(out, "192.0.2.1, 192.0.2.2, router.example") || strings.Count(out, "router.example") != 1 || !strings.Contains(out, "| unmatched |") || strings.Contains(out, "=unmatched") {
			t.Fatalf("redundant address/identity display: %s", out)
		}
		if strings.Count(out, "shared") != 1 || strings.Count(out, "  Limitation: first two only") != 2 || strings.Count(out, "  Limitation: third only") != 1 {
			t.Fatalf("caveat scope lost: %s", out)
		}
	}
	var encoded bytes.Buffer
	if e := Render(&encoded, r, "json"); e != nil {
		t.Fatal(e)
	}
	var got Report
	if e := json.Unmarshal(encoded.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	after, e := json.Marshal(got)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("compaction changed JSON facts or caveats")
	}
	// Mixed matches must retain which address is unmatched and all ambiguity candidates.
	h.Matches["192.0.2.2"] = trace.Match{Method: "addresses-ambiguous", Candidates: []string{"leaf-a", "leaf-b"}}
	var mixed bytes.Buffer
	if e := Render(&mixed, r, "text"); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(mixed.String(), "192.0.2.1=unmatched; 192.0.2.2=leaf-a, leaf-b (addresses-ambiguous)") {
		t.Fatal("mixed identity attribution lost")
	}
	h.Matches["192.0.2.2"] = trace.Match{Device: "leaf", Method: "address", Candidates: []string{"leaf"}}
	var matched bytes.Buffer
	if e := Render(&matched, r, "text"); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(matched.String(), "192.0.2.2=leaf (address)") {
		t.Fatal("matched identity lost")
	}
}
