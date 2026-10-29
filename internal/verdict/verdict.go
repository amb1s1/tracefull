// Package verdict interprets saved observations; it never establishes a forwarding cause.
package verdict

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/amb1s1/tracefull/internal/trace"
)

// MinimumSamples is a comparability floor, not a statistical confidence guarantee.
const MinimumSamples = 100
const MinimumLoss = 1.0
const PersistenceRatio = 0.5
const DefaultLatencyJump = 15 * time.Millisecond

type Finding struct {
	Kind        string   `json:"kind"`
	Hop         int      `json:"hop"`
	Flow        string   `json:"flow"`
	Severity    string   `json:"severity"`
	Text        string   `json:"text"`
	Rule        string   `json:"rule"`
	Evidence    []string `json:"evidence"`
	Limitations []string `json:"limitations"`
}
type Rule struct{ Kind, Severity, Text, Example string }

func Rules() []Rule {
	return []Rule{
		{"unreachable", "problem", "No destination response in a completed measurement; filtering or a return-path problem remain possible.", "Completed trace ends without a destination reply; test the actual service separately."},
		{"hop-limit", "warn", "The configured hop limit was reached; destination reachability is unresolved.", "Header says 30 hops max and TTL 30 still responds."},
		{"phantom-loss", "info", "Intermediate response loss does not persist downstream; reply handling is a possible explanation.", "Hop 2 loses 60% of 100 probes, later destination loses 0%."},
		{"silent-hop", "info", "A silent TTL is followed by replies; this is consistent with forwarding without visible responses.", "TTL 2 is silent; TTL 3 answers."},
		{"real-loss", "problem", "Response-loss pattern persists to the destination; the forward path, return path and reply handling remain candidates.", "At least 100 samples each: 8%, 6%, 5% loss from TTL 2 to destination."},
		{"dest-only-loss", "problem", "Only the destination has measured response loss; service traffic and destination reply limiting need independent checks.", "100 samples each: 0%, 0%, 8%."},
		{"latency-added", "warn", "RTT rise persists downstream; RTT includes reply generation and the unseen return path.", "Average RTT rises from 5 to 25 ms and stays above 15 ms."},
		{"latency-control-plane", "info", "A transient RTT rise is consistent with response handling or return-path variation.", "Average RTT is 5, 40, 6 ms."},
		{"jitter", "info", "RTT spread is large; queueing, reply generation and path variation remain possible.", "Best 5, average 10, worst 80 ms."},
		{"ecmp", "warn", "Multiple responders or flow paths were observed; this does not prove simultaneous ECMP.", "One TTL contains two response addresses."},
		{"mtu-drop", "problem", "Imported measured PMTU decreases; the observation does not isolate a failing link.", "Tracepath PMTU falls from 1500 to 1400 bytes."},
		{"asymmetry", "info", "Imported tracepath asymmetry hint is an inference, not a measured reverse path.", "Tracepath prints asymm 3."},
		{"icmp-error", "problem", "A probe received an explicit ICMP error; other flows may still work.", "Traceroute records !H or !X."},
		{"nat", "info", "Imported Dublin NAT identifier changes; packet rewriting is a candidate explanation.", "Adjacent observations have different nat_id values."},
	}
}
func Evaluate(t trace.Trace, jump time.Duration) []Finding {
	if jump <= 0 {
		jump = DefaultLatencyJump
	}
	out := []Finding{}
	if len(t.Paths) > 0 {
		for _, p := range t.Paths {
			pt := t
			pt.Paths = nil
			pt.Hops = p.Hops
			pt.Reached = p.Reached
			pt.Complete = false
			out = append(out, evaluate(pt, jump, p.ID)...)
		}
		if distinctPaths(t.Paths) && len(t.Hops) > 0 {
			out = append(out, finding("ecmp", t.Hops[0], "", t.Hops...))
		}
		return ordered(out)
	}
	return ordered(evaluate(t, jump, ""))
}
func finding(kind string, h trace.Hop, flow string, support ...trace.Hop) Finding {
	f := Finding{Kind: kind, Hop: h.TTL, Flow: flow, Rule: "v0.1/" + kind, Evidence: []string{}, Limitations: []string{"Saved observations do not establish current state or a forwarding cause; flow identity and measurement window may be unknown."}}
	for _, r := range Rules() {
		if r.Kind == kind {
			f.Severity = r.Severity
			f.Text = r.Text
			break
		}
	}
	for _, s := range append([]trace.Hop{h}, support...) {
		for _, e := range s.Evidence {
			f.Evidence = trace.AddUnique(f.Evidence, e)
		}
	}
	return f
}

// Each rule has one evaluation function; ordering follows the public rule table.
func evaluate(t trace.Trace, jump time.Duration, flow string) []Finding {
	out := []Finding{}
	hs := t.Hops
	if len(hs) == 0 {
		return out
	}
	last := len(hs) - 1
	if t.Reached == nil || !*t.Reached {
		if t.MaxHops != nil && hs[last].TTL == *t.MaxHops && len(hs[last].Addresses) > 0 {
			out = append(out, finding("hop-limit", hs[last], flow))
		} else if t.Reached != nil && t.Complete {
			out = append(out, finding("unreachable", hs[last], flow))
		}
	}
	comparable := lossComparable(t)
	realEmitted := false
	for i, h := range hs {
		for _, kind := range localRules(hs, i) {
			out = append(out, finding(kind, h, flow, hs...))
		}
		if comparable {
			for _, kind := range lossRules(hs, i, &realEmitted) {
				out = append(out, finding(kind, h, flow, hs...))
			}
		}
		if kind := latencyRule(t, i, jump); kind != "" {
			out = append(out, finding(kind, h, flow, hs...))
		}
	}
	return out
}
func lossComparable(t trace.Trace) bool {
	hs := t.Hops
	last := len(hs) - 1
	if t.Reached == nil || !*t.Reached || len(hs[last].Addresses) != 1 || hs[last].Addresses[0] != t.DestAddr {
		return false
	}
	for i, h := range hs {
		if h.Sent == nil || *h.Sent < MinimumSamples || h.Loss == nil || len(h.Addresses) > 1 || i > 0 && h.TTL != hs[i-1].TTL+1 {
			return false
		}
	}
	return true
}
func localRules(hs []trace.Hop, i int) []string {
	h := hs[i]
	kinds := []string{}
	if len(h.Addresses) > 1 {
		kinds = append(kinds, "ecmp")
	}
	silent := h.Loss != nil && *h.Loss == 100
	for _, n := range h.Notes {
		silent = silent || strings.HasPrefix(n, "no reply;")
	}
	if silent && len(h.Addresses) == 0 && len(h.Names) == 0 {
		for _, later := range hs[i+1:] {
			if len(later.Addresses) > 0 || len(later.Names) > 0 {
				kinds = append(kinds, "silent-hop")
				break
			}
		}
	}
	for _, n := range h.Notes {
		if strings.HasPrefix(n, "!") {
			kinds = append(kinds, "icmp-error")
			break
		}
	}
	for _, tag := range h.Tags {
		if tag == "asymmetry-hint" {
			kinds = append(kinds, "asymmetry")
		}
		if tag == "nat-hint" {
			kinds = append(kinds, "nat")
		}
	}
	if i > 0 && h.PMTU != nil && hs[i-1].PMTU != nil && *h.PMTU < *hs[i-1].PMTU {
		kinds = append(kinds, "mtu-drop")
	}
	if h.Best != nil && h.Avg != nil && h.Worst != nil && *h.Worst-*h.Best > math.Max(50, 5*(*h.Avg-*h.Best)) {
		kinds = append(kinds, "jitter")
	}
	return kinds
}
func lossRules(hs []trace.Hop, i int, realEmitted *bool) []string {
	h := hs[i]
	last := len(hs) - 1
	if *h.Loss <= 0 {
		return nil
	}
	// At the overlapping 50% boundary, prefer the narrower non-persistence observation.
	if i < last && *hs[last].Loss <= *h.Loss*PersistenceRatio {
		return []string{"phantom-loss"}
	}
	if i == last {
		only := true
		for _, p := range hs[:last] {
			only = only && *p.Loss == 0
		}
		if only {
			return []string{"dest-only-loss"}
		}
		return nil
	}
	if !*realEmitted && *h.Loss >= MinimumLoss {
		persists := true
		for _, p := range hs[i+1:] {
			persists = persists && *p.Loss >= *h.Loss*PersistenceRatio
		}
		if persists {
			*realEmitted = true
			return []string{"real-loss"}
		}
	}
	return nil
}
func latencyRule(t trace.Trace, i int, jump time.Duration) string {
	hs := t.Hops
	h := hs[i]
	last := len(hs) - 1
	if i == 0 || h.TTL != hs[i-1].TTL+1 || h.Avg == nil || hs[i-1].Avg == nil || len(h.Addresses) > 1 || len(hs[i-1].Addresses) > 1 {
		return ""
	}
	rise := *h.Avg - *hs[i-1].Avg
	if rise < float64(jump)/float64(time.Millisecond) {
		return ""
	}
	floor := *hs[i-1].Avg + rise/2
	persistent := t.Reached != nil && *t.Reached
	for j := i + 1; j < len(hs); j++ {
		persistent = persistent && hs[j].Avg != nil && len(hs[j].Addresses) <= 1 && hs[j].TTL == hs[j-1].TTL+1
		if hs[j].Avg != nil {
			persistent = persistent && *hs[j].Avg >= floor
		}
	}
	if persistent {
		return "latency-added"
	}
	if i < last && hs[i+1].TTL == h.TTL+1 && hs[i+1].Avg != nil && *hs[i+1].Avg < floor {
		return "latency-control-plane"
	}
	return ""
}
func distinctPaths(paths []trace.Path) bool {
	seen := map[string]bool{}
	for _, p := range paths {
		var b strings.Builder
		for _, h := range p.Hops {
			fmt.Fprintf(&b, "%d=%s;", h.TTL, strings.Join(h.Addresses, ","))
		}
		seen[b.String()] = true
	}
	return len(seen) > 1
}
func ordered(fs []Finding) []Finding {
	order := map[string]int{}
	for i, r := range Rules() {
		order[r.Kind] = i
	}
	sort.SliceStable(fs, func(i, j int) bool { return order[fs[i].Kind] < order[fs[j].Kind] })
	return fs
}
func Notes(t trace.Trace) []string {
	for _, h := range t.Hops {
		if h.Sent == nil || *h.Sent < MinimumSamples {
			return []string{fmt.Sprintf("Loss classification withheld where comparable per-hop samples are unknown or below %d; this threshold is not a confidence guarantee.", MinimumSamples)}
		}
	}
	if len(t.Hops) > 0 && !lossComparable(t) {
		return []string{"Loss comparison withheld: destination response, contiguous samples or responder identity is unresolved."}
	}
	return []string{}
}
