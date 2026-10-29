package paris

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/parse/traceroute"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string { return "paris" }
func (Parser) Detect(b []byte) bool {
	return bytes.Contains(b, []byte("traceroute [(")) || bytes.Contains(b, []byte("paris-traceroute"))
}

var endpoints = regexp.MustCompile(`\[\(([^)]+)\) -> \(([^)]+)\)\]`)
var summary = regexp.MustCompile(`([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+) ms`)

// Parse retains the aggregate summary beside the corresponding TTL; no per-flow data is invented.
func (Parser) Parse(b []byte) (trace.Trace, error) {
	// Exhaustive summaries expose aggregate RTT, not per-flow samples.
	lines := strings.Split(string(b), "\n")
	converted := append([]string(nil), lines...)
	for i, l := range lines {
		if summary.MatchString(l) {
			converted[i] = summary.ReplaceAllString(l, "$2 ms")
		}
	}
	t, e := traceroute.ParseText([]byte(strings.Join(converted, "\n")), false)
	if e != nil {
		return t, e
	}
	if m := endpoints.FindStringSubmatch(string(b)); m != nil {
		for i, s := range m[1:] {
			n := strings.LastIndex(s, ":")
			if n < 0 {
				continue
			}
			a := strings.Trim(s[:n], "[]")
			if i == 0 {
				t.SourceAddr = a
			} else {
				t.DestAddr = a
			}
		}
	}
	for i := range t.Hops {
		h := &t.Hops[i]
		h.Sent = nil
		h.Lost = nil
		h.Loss = nil
		for _, line := range lines {
			m := parseutil.HopLine.FindStringSubmatch(line)
			if m == nil || parseutil.Int(m[1]) != h.TTL {
				continue
			}
			if v := summary.FindStringSubmatch(line); v != nil {
				ptrs := []**float64{&h.Best, &h.Avg, &h.Worst, &h.Stdev}
				for j, p := range ptrs {
					f, err := parseutil.Float(v[j+1])
					if err != nil {
						return t, fmt.Errorf("paris summary: %w", err)
					}
					*p = trace.Ptr(f)
				}
			}
		}
	}
	t.Notes = append(t.Notes, "Paris aggregate output does not retain per-flow sample counts; loss comparison unavailable.")
	return t, nil
}
