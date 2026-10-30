package collect

import (
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/evidence"
	"github.com/amb1s1/tracefull/internal/trace"
)

func TestComparisonQualifications(t *testing.T) {
	// Synthetic facts deliberately exercise comparisons absent from the real capture.
	yes := true
	mtu := 1500
	low := uint64(1400)
	p := Prepared{Plan: Plan{Destination: "192.0.2.9", Context: "default"}}
	p.Trace.Hops = []trace.Hop{{TTL: 1, Addresses: []string{"192.0.2.1"}, Matches: map[string]trace.Match{"192.0.2.1": {Device: "a"}}, PMTU: &mtu, Evidence: []string{"line:2"}}, {TTL: 2, Addresses: []string{"192.0.2.2"}, Evidence: []string{"line:3"}}}
	f := driver.Empty()
	f.Addresses = []driver.Address{{Address: "192.0.2.1", Interface: "lo"}}
	f.Routes = []driver.Route{{Prefix: "0.0.0.0/0", Context: "default", Selected: &yes, Installed: &yes, NextHops: []driver.NextHop{{Address: "192.0.2.8", Interface: "swp1", Active: &yes, Installed: &yes}}}}
	f.Counters = []driver.Counter{{Interface: "swp1", MTUBytes: &low}}
	d := []DeviceResult{{Name: "a", Commands: []CommandResult{{Stdout: &evidence.File{Path: "raw"}, Facts: &f}}}}
	got := comparisons(p, d)
	if len(got) != 2 || got[0].Kind != "path-mismatch" || got[1].Kind != "mtu-drop" {
		t.Fatal(got)
	}
	f.Routes[0].NextHops[0].Address = "192.0.2.2"
	if got = comparisons(p, d); len(got) != 1 || got[0].Kind != "mtu-drop" {
		t.Fatal(got)
	}
	f.Routes[0].NextHops[0].Address = "fe80::1"
	if len(comparisons(p, d)) != 1 {
		t.Fatal("link-local compared with global responder")
	}
	f.Addresses[0].Master = "mgmt"
	if len(comparisons(p, d)) != 0 {
		t.Fatal("management context compared to default")
	}
	f.Addresses[0].Master = ""
	f.Routes[0].Selected = nil
	if len(comparisons(p, d)) != 0 {
		t.Fatal("unknown route selection compared")
	}
}
