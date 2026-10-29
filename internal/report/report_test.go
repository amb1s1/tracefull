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
