package parseutil

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/trace"
)

var HopLine = regexp.MustCompile(`^\s*(\d+)(?::|\s)\s*(.*)$`)
var RTT = regexp.MustCompile(`(<)?([0-9]+(?:\.[0-9]+)?)\s*ms`)
var Annotation = regexp.MustCompile(`![A-Za-z0-9]+`)
var Limit = regexp.MustCompile(`(?i)(\d+) hops? (?:max|maximum)|maximum of (\d+) hops`)

func Int(s string) int { n, _ := strconv.Atoi(s); return n }
func Float(s string) (float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return v, nil
}
func Addresses(s string) []string {
	out := []string{}
	for _, f := range strings.Fields(s) {
		f = strings.Trim(f, "()[],")
		if a, e := netip.ParseAddr(f); e == nil {
			out = trace.AddUnique(out, a.Unmap().String())
		}
	}
	return out
}
func Header(t *trace.Trace, line string) {
	if m := Limit.FindStringSubmatch(line); m != nil {
		t.MaxHops = trace.Ptr(Int(m[1] + m[2]))
	}
	if strings.HasPrefix(line, "traceroute to ") || strings.HasPrefix(line, "traceroute6 to ") {
		f := strings.Fields(line)
		t.Dest = strings.TrimSuffix(f[2], ",")
		a := Addresses(line)
		if len(a) > 0 {
			t.DestAddr = a[0]
		}
	}
	if strings.HasPrefix(line, "Tracing route to ") {
		f := strings.Fields(line)
		if len(f) > 3 {
			t.Dest = f[3]
		}
		a := Addresses(line)
		if len(a) > 0 {
			t.DestAddr = a[0]
		}
	}
	if t.DestAddr == "" {
		a := Addresses(line)
		if strings.HasPrefix(line, "over a maximum") && len(a) > 0 {
			t.DestAddr = a[0]
		}
	}
}
func Hop(ttl int, body, ref string) (trace.Hop, error) {
	h := trace.NewHop(ttl, ref)
	h.Addresses = Addresses(body)
	matches := RTT.FindAllStringSubmatch(body, -1)
	rtts := []float64{}
	censored := false
	for _, m := range matches {
		v, e := Float(m[2])
		if e != nil {
			return h, e
		}
		rtts = append(rtts, v)
		censored = censored || m[1] != ""
	}
	lost := strings.Count(body, "*")
	if strings.Contains(body, "Request timed out") && lost == 0 {
		lost = 3
	}
	if len(rtts)+lost > 0 {
		h.Samples(rtts, lost)
	}
	if censored {
		h.Best = nil
		h.Avg = nil
		h.Worst = nil
		h.Stdev = nil
		h.Notes = append(h.Notes, "RTT upper bounds (<) retained in source; exact statistics unknown.")
	}
	h.Notes = append(h.Notes, Annotation.FindAllString(body, -1)...)
	fields := strings.Fields(body)
	for i, f := range fields {
		if (strings.HasPrefix(f, "(") || strings.HasPrefix(f, "[")) && i > 0 && len(Addresses(f)) > 0 {
			name := fields[i-1]
			if len(Addresses(name)) == 0 {
				h.Names = trace.AddUnique(h.Names, name)
			}
		}
	}
	if h.Sent == nil && len(h.Addresses) == 0 {
		return h, fmt.Errorf("hop %d has no recognizable measurements", ttl)
	}
	return h, nil
}
