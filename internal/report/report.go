// Package report renders precomputed reports without deriving findings.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/trace"
	"github.com/amb1s1/tracefull/internal/verdict"
)

type Source struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}
type Report struct {
	SchemaVersion string `json:"schema_version"`
	Version       string `json:"tracefull_version"`
	trace.Trace
	Source        Source             `json:"source"`
	DNSObservedAt *string            `json:"dns_observed_at"`
	Findings      []verdict.Finding  `json:"findings"`
	Devices       []inventory.Device `json:"devices"`
}

func Safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func Escape(s string) string {
	s = Safe(s)
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\\", "&#92;", "|", "&#124;", "`", "&#96;", "*", "&#42;", "_", "&#95;", "[", "&#91;", "]", "&#93;", "#", "&#35;")
	return r.Replace(s)
}
func Number[T int | float64](n *T) string {
	if n == nil {
		return "?"
	}
	if v, ok := any(*n).(float64); ok {
		out := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 3, 64), "0"), ".")
		if out == "-0" {
			return "0"
		}
		return out
	}
	return fmt.Sprint(*n)
}

type TableRenderer func(headers []string, rows [][]string) string

// FindingLabel describes the observation without changing machine-readable rule identifiers.
func FindingLabel(kind string) string {
	if kind == "ecmp" {
		return "multiple responders / paths"
	}
	return kind
}

func Render(w io.Writer, r Report, format string) error { return render(w, r, format, nil) }

// RenderText accepts a terminal table view while retaining the same report contents.
func RenderText(w io.Writer, r Report, table TableRenderer) error { return render(w, r, "text", table) }
func render(w io.Writer, r Report, format string, table TableRenderer) error {
	if format == "json" {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		return e.Encode(r)
	}
	if format != "text" && format != "markdown" {
		return fmt.Errorf("unknown format %q", format)
	}
	var b strings.Builder
	escape := Safe
	if format == "markdown" {
		escape = Escape
		b.WriteString("# tracefull report\n\n")
	}
	fmt.Fprintf(&b, "Destination: %s (%s) | reached: %s | tool: %s\n", escape(r.Dest), escape(r.DestAddr), boolValue(r.Reached), escape(known(r.Probe.Tool)))
	fmt.Fprintf(&b, "Source: %s | SHA-256: %s\n", escape(r.Source.Name), r.Source.SHA256)
	fmt.Fprintf(&b, "Measurement window (UTC): %s to %s\n", timeValue(r.StartedAt), timeValue(r.EndedAt))
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "Note: %s\n", escape(n))
	}
	renderHops(&b, r.Hops, format, escape, table)
	for _, p := range r.Paths {
		fmt.Fprintf(&b, "\nFlow %s (reached: %s; UDP ports %s to %s; UTC %s to %s)\n", escape(p.ID), boolValue(p.Reached), Number(p.SourcePort), Number(p.DestPort), timeValue(p.StartedAt), timeValue(p.EndedAt))
		renderHops(&b, p.Hops, format, escape, table)
	}
	for _, d := range r.Devices {
		fmt.Fprintf(&b, "\nDevice %s | platform: %s | role: %s | site: %s\nNotes: %s\n", escape(d.Name), escape(d.Platform), escape(d.Role), escape(d.Site), escape(d.Notes))
	}
	b.WriteString("\nFindings\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "- %s %s at hop %d", f.Severity, escape(FindingLabel(f.Kind)), f.Hop)
		if f.Flow != "" {
			fmt.Fprintf(&b, " flow %s", escape(f.Flow))
		}
		fmt.Fprintf(&b, ": %s [%s]\n  Evidence: %s\n", escape(f.Text), f.Rule, escape(strings.Join(f.Evidence, ", ")))
		for _, n := range f.Limitations {
			fmt.Fprintf(&b, "  Limitation: %s\n", escape(n))
		}
	}
	if len(r.Findings) == 0 {
		b.WriteString("No findings from the available observations.\n")
	}
	_, e := io.WriteString(w, b.String())
	return e
}
func boolValue(v *bool) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprint(*v)
}
func renderHops(b *strings.Builder, hops []trace.Hop, format string, escape func(string) string, table TableRenderer) {
	headers := []string{"hop", "address / name", "loss %", "sent", "avg ms", "best ms", "worst ms", "PMTU bytes", "identity"}
	rows := [][]string{}
	for _, h := range hops {
		ids := []string{}
		for _, a := range h.Addresses {
			if m, ok := h.Matches[a]; ok {
				ids = append(ids, a+"="+m.Device+" ("+m.Method+" "+strings.Join(m.Candidates, ",")+")")
			}
		}
		rows = append(rows, []string{fmt.Sprint(h.TTL), escape(strings.Join(append(append([]string{}, h.Addresses...), h.Names...), ", ")), Number(h.Loss), Number(h.Sent), Number(h.Avg), Number(h.Best), Number(h.Worst), Number(h.PMTU), escape(strings.Join(ids, "; "))})
	}
	b.WriteString("\n")
	if table != nil {
		b.WriteString(table(headers, rows))
		b.WriteString("\n")
	} else {
		fmt.Fprintf(b, "| %s |\n", strings.Join(headers, " | "))
		if format == "markdown" {
			b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		}
		for _, row := range rows {
			fmt.Fprintf(b, "| %s |\n", strings.Join(row, " | "))
		}
	}
	b.WriteString("\n")
	for _, h := range hops {
		for _, a := range h.Addresses {
			if n := h.RDNS[a]; len(n) > 0 {
				fmt.Fprintf(b, "  rDNS %s: %s\n", a, escape(strings.Join(n, ", ")))
			}
			if n := h.ASN[a]; len(n) > 0 {
				fmt.Fprintf(b, "  ASN %s: %s\n", a, escape(strings.Join(n, ", ")))
			}
		}
		for _, n := range append(append([]string{}, h.Tags...), h.Notes...) {
			fmt.Fprintf(b, "  Hop %d: %s\n", h.TTL, escape(n))
		}
	}
}

func timeValue(s *string) string {
	if s == nil {
		return "unknown"
	}
	return Safe(*s)
}

func known(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
