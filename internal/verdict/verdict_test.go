package verdict

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/trace"
)

func base(losses ...float64) trace.Trace {
	t := trace.Trace{Reached: trace.Ptr(true), Complete: true}
	for i, l := range losses {
		h := trace.NewHop(i+1, fmt.Sprintf("line:%d", i+1))
		h.AddAddress(fmt.Sprintf("192.0.2.%d", i+1))
		h.Sent = trace.Ptr(100)
		h.Loss = trace.Ptr(l)
		h.Best = trace.Ptr(1.0)
		h.Avg = trace.Ptr(2.0)
		h.Worst = trace.Ptr(3.0)
		t.Hops = append(t.Hops, h)
	}
	t.DestAddr = fmt.Sprintf("192.0.2.%d", len(losses))
	return t
}
func has(fs []Finding, k string) bool {
	for _, f := range fs {
		if f.Kind == k {
			return true
		}
	}
	return false
}
func TestRules(t *testing.T) {
	tests := []struct {
		name, want, absent string
		setup              func() trace.Trace
	}{
		{"phantom", "phantom-loss", "real-loss", func() trace.Trace { return base(60, 0, 0) }},
		{"persistent", "real-loss", "dest-only-loss", func() trace.Trace { return base(0, 8, 5) }},
		{"dest", "dest-only-loss", "real-loss", func() trace.Trace { return base(0, 0, 8) }},
		{"threshold", "phantom-loss", "real-loss", func() trace.Trace { return base(1, 0.5, 0.5) }},
		{"below", "", "real-loss", func() trace.Trace { return base(0.9, 0.9) }},
		{"low", "", "real-loss", func() trace.Trace { v := base(8, 8); v.Hops[1].Sent = trace.Ptr(99); return v }},
		{"unknown", "", "real-loss", func() trace.Trace { v := base(8, 8); v.Hops[1].Loss = nil; return v }},
		{"gap", "", "real-loss", func() trace.Trace { v := base(8, 8); v.Hops[1].TTL = 3; return v }},
		{"braid", "ecmp", "real-loss", func() trace.Trace { v := base(8, 8); v.Hops[0].AddAddress("192.0.2.100"); return v }},
		{"silent", "silent-hop", "", func() trace.Trace { v := base(100, 0); v.Hops[0].Addresses = nil; return v }},
		{"unreachable", "unreachable", "hop-limit", func() trace.Trace { v := base(0, 0); v.Reached = trace.Ptr(false); return v }},
		{"limit", "hop-limit", "unreachable", func() trace.Trace { v := base(0, 0); v.Reached = trace.Ptr(false); v.MaxHops = trace.Ptr(2); return v }},
		{"truncated", "", "unreachable", func() trace.Trace { v := base(0, 0); v.Reached = nil; v.Complete = false; return v }},
		{"latency", "latency-added", "latency-control-plane", func() trace.Trace {
			v := base(0, 0, 0)
			v.Hops[1].Avg = trace.Ptr(30.0)
			v.Hops[2].Avg = trace.Ptr(20.0)
			return v
		}},
		{"transient", "latency-control-plane", "latency-added", func() trace.Trace { v := base(0, 0, 0); v.Hops[1].Avg = trace.Ptr(30.0); return v }},
		{"missing-rtt", "", "latency-added", func() trace.Trace { v := base(0, 0, 0); v.Hops[1].Avg = trace.Ptr(30.0); v.Hops[2].Avg = nil; return v }},
		{"jitter", "jitter", "", func() trace.Trace { v := base(0); v.Hops[0].Worst = trace.Ptr(100.0); return v }},
		{"mtu", "mtu-drop", "", func() trace.Trace {
			v := base(0, 0)
			v.Hops[0].PMTU = trace.Ptr(1500)
			v.Hops[1].PMTU = trace.Ptr(1400)
			return v
		}},
		{"asymmetry", "asymmetry", "", func() trace.Trace { v := base(0); v.Hops[0].Tags = []string{"asymmetry-hint"}; return v }},
		{"nat", "nat", "", func() trace.Trace { v := base(0); v.Hops[0].Tags = []string{"nat-hint"}; return v }},
		{"error", "icmp-error", "", func() trace.Trace { v := base(0); v.Hops[0].Notes = []string{"!X"}; return v }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := Evaluate(tc.setup(), 15*time.Millisecond)
			if tc.want != "" && !has(fs, tc.want) {
				t.Fatalf("missing %s: %+v", tc.want, fs)
			}
			if tc.absent != "" && has(fs, tc.absent) {
				t.Fatalf("unexpected %s", tc.absent)
			}
			for _, f := range fs {
				if f.Text == "" || len(f.Evidence) == 0 || len(f.Limitations) == 0 {
					t.Fatal("unattributed finding")
				}
			}
		})
	}
}
func TestSeparateFlows(t *testing.T) {
	v := base(0, 0)
	a := base(50, 0)
	b := base(0, 50)
	b.Hops[0].Addresses = []string{"192.0.2.50"}
	v.Paths = []trace.Path{{ID: "a", Hops: a.Hops, Reached: a.Reached}, {ID: "b", Hops: b.Hops, Reached: b.Reached}}
	fs := Evaluate(v, 0)
	if !has(fs, "ecmp") {
		t.Fatal("missing multipath")
	}
	for _, f := range fs {
		if f.Kind == "real-loss" {
			t.Fatal("mixed flows")
		}
		if f.Kind == "dest-only-loss" && f.Flow != "b" {
			t.Fatal("wrong attribution")
		}
	}
}
func TestEmptyAndNotes(t *testing.T) {
	if len(Evaluate(trace.Trace{}, 0)) != 0 {
		t.Fatal("empty findings")
	}
	v := base(0)
	if len(Notes(v)) != 0 {
		t.Fatal("unexpected note")
	}
	v.Hops[0].Sent = nil
	if len(Notes(v)) == 0 {
		t.Fatal("missing sample limit")
	}
}

func TestIdenticalFlowsAndUnknownSilence(t *testing.T) {
	v := base(0, 0)
	v.Paths = []trace.Path{{ID: "a", Hops: v.Hops, Reached: v.Reached}, {ID: "b", Hops: v.Hops, Reached: v.Reached}}
	if has(Evaluate(v, 0), "ecmp") {
		t.Fatal("identical paths are not ECMP evidence")
	}
	v.Paths = nil
	v.Hops[0].Addresses = nil
	v.Hops[0].Loss = nil
	if has(Evaluate(v, 0), "silent-hop") {
		t.Fatal("unknown response count became silence")
	}
}

func TestMultipleResponderEvidence(t *testing.T) {
	v := base(0, 0, 0)
	v.Hops[1].AddAddress("192.0.2.20")
	v.Hops[1].Evidence = []string{"line:2", "line:3"}
	v.Hops[2].Evidence = []string{"line:4"}
	fs := Evaluate(v, 0)
	if len(fs) != 1 {
		t.Fatalf("expected one observation, got %+v", fs)
	}
	f := fs[0]
	if f.Kind != "ecmp" || f.Rule != "v0.1/ecmp" || f.Severity != "warn" || f.Hop != 2 {
		t.Fatalf("finding contract changed: %+v", f)
	}
	if !reflect.DeepEqual(f.Evidence, []string{"line:2", "line:3"}) {
		t.Fatalf("unrelated evidence included: %v", f.Evidence)
	}
	if !strings.Contains(f.Text, "Multiple responders at this TTL") || !strings.Contains(f.Text, "neither ECMP nor a network fault") {
		t.Fatal(f.Text)
	}
	// A comparison across recorded flow paths still needs evidence beyond one TTL.
	v.Paths = []trace.Path{{ID: "a", Hops: base(0, 0, 0).Hops}, {ID: "b", Hops: v.Hops}}
	fs = Evaluate(v, 0)
	comparison := fs[len(fs)-1]
	if comparison.Flow != "" || !reflect.DeepEqual(comparison.Evidence, []string{"line:1", "line:2", "line:3", "line:4"}) {
		t.Fatalf("flow comparison evidence lost: %+v", comparison)
	}
}
