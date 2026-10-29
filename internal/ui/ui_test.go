package ui

import (
	"bytes"
	"github.com/amb1s1/tracefull/internal/report"
	"github.com/amb1s1/tracefull/internal/trace"
	"github.com/amb1s1/tracefull/internal/verdict"
	"strings"
	"testing"
)

func TestThemeAndPlain(t *testing.T) {
	h := trace.NewHop(1, "line:1")
	h.AddAddress("192.0.2.1")
	r := report.Report{Trace: trace.Trace{Hops: []trace.Hop{h}, Dest: "unsafe\x1b[31m"}, Findings: []verdict.Finding{{Kind: "test", Severity: "problem", Text: "qualified"}}}
	for _, color := range []bool{false, true} {
		var b bytes.Buffer
		if e := Report(&b, r, "text", color); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(b.String(), "192.0.2.1") || strings.Count(b.String(), "192.0.2.1") != 1 {
			t.Fatal("table missing or duplicated")
		}
		if strings.Contains(b.String(), "unsafe\x1b") {
			t.Fatal("untrusted ANSI")
		}
		if !color && strings.Contains(b.String(), "\x1b") {
			t.Fatal("ANSI in plain view")
		}
	}
	t.Setenv("NO_COLOR", "1")
	if Color(&bytes.Buffer{}, false) {
		t.Fatal("color in pipe")
	}
}
func TestExplainView(t *testing.T) {
	for _, color := range []bool{false, true} {
		var b bytes.Buffer
		if e := Markdown(&b, "# Rules\n\nEvidence is limited.\n", color); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(b.String(), "Rules") {
			t.Fatal("missing explanation")
		}
	}
}
