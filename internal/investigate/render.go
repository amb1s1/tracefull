package investigate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/amb1s1/tracefull/internal/report"
)

func RenderPlan(w io.Writer, p Plan, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(p)
	}
	escape := report.Safe
	if format == "markdown" {
		escape = report.Escape
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Dry run for %s — no DNS lookups or tools executed.\n", escape(p.Target))
	for _, c := range p.Commands {
		argv, _ := json.Marshal(append([]string{c.Executable}, c.Args...))
		fmt.Fprintf(&b, "- %s: %s", c.Tool, c.Status)
		if c.Status == "planned" {
			fmt.Fprintf(&b, " argv=%s", escape(string(argv)))
		} else {
			fmt.Fprintf(&b, " — %s", escape(c.Reason))
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "Limits: %.0fs overall, %.0fs per tool, %d bytes per output stream.\n", p.TimeoutSeconds, p.ToolTimeoutSeconds, p.MaxOutputBytes)
	_, err := io.WriteString(w, b.String())
	return err
}

func Render(w io.Writer, r Report, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	if format != "text" && format != "markdown" {
		return fmt.Errorf("invalid format %q", format)
	}
	escape := report.Safe
	if format == "markdown" {
		escape = report.Escape
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Investigation: %s (%s) | %s\nExecution window (UTC): %s to %s\nEvidence: %s\n", escape(r.Plan.Target), escape(r.Plan.Address), r.Status, r.StartedAt, r.EndedAt, escape(r.EvidenceDir))
	if r.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", escape(r.Error))
	}
	b.WriteString("\nTool coverage\n")
	for _, result := range r.Results {
		fmt.Fprintf(&b, "- %s: %s", result.Tool, result.Status)
		if result.Reason != "" {
			fmt.Fprintf(&b, " — %s", escape(result.Reason))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nMeasurements remain separate by tool, probe method and execution window; samples are not combined.\n")
	for _, result := range r.Results {
		if result.Report == nil {
			continue
		}
		fmt.Fprintf(&b, "\n## %s observations\n\n", result.Tool)
		if err := report.Render(&b, *result.Report, format); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
