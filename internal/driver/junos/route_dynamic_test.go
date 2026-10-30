package junos

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestISISRouteCapture(t *testing.T) {
	f, err := (Driver{}).Parse("route", fixture(t, "route4-isis-detail"))
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	base := "json:/route-information/0/route-table/0/rt/0/rt-entry/0"
	want := []driver.Route{{Prefix: "192.0.2.10/32", Protocol: "IS-IS", Context: "default", Selected: &yes, NextHops: []driver.NextHop{
		{Address: "198.51.100.97", Interface: "et-0/0/0.0", Active: &yes, Evidence: base + "/nh/0"},
		{Address: "198.51.100.99", Interface: "et-0/1/0.0", Evidence: base + "/nh/1"},
	}, Evidence: base}}
	if !reflect.DeepEqual(f.Routes, want) {
		t.Fatalf("got %+v; want %+v", f.Routes, want)
	}
}

func mutateISIS(t *testing.T, change func(node, []node)) []byte {
	t.Helper()
	var root node
	if err := json.Unmarshal(fixture(t, "route4-isis-detail"), &root); err != nil {
		t.Fatal(err)
	}
	groups := root.list("route-information")
	tables := groups[0].list("route-table")
	routes := tables[0].list("rt")
	entries := routes[0].list("rt-entry")
	hops := entries[0].list("nh")
	change(entries[0], hops)
	// Rebuild the decoded containers; changes remain synthetic test mutations.
	pack := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	entries[0]["nh"] = pack(hops)
	routes[0]["rt-entry"] = pack(entries)
	tables[0]["rt"] = pack(routes)
	groups[0]["route-table"] = pack(tables)
	root["route-information"] = pack(groups)
	return pack(root)
}

func TestMalformedRouteSelection(t *testing.T) {
	for name, change := range map[string]func(node, []node){
		"false string marker": func(_ node, h []node) { h[0]["selected-next-hop"] = json.RawMessage(`[{"data":"false"}]`) },
		"true string marker":  func(_ node, h []node) { h[0]["selected-next-hop"] = json.RawMessage(`[{"data":"true"}]`) },
		"empty marker":        func(_ node, h []node) { h[0]["selected-next-hop"] = json.RawMessage(`[{}]`) },
		"duplicate marker": func(_ node, h []node) {
			h[0]["selected-next-hop"] = json.RawMessage(`[{"data":[null]},{"data":[null]}]`)
		},
		"unknown tag":   func(e node, _ []node) { e["active-tag"] = json.RawMessage(`[{"data":"?"}]`) },
		"duplicate tag": func(e node, _ []node) { e["active-tag"] = json.RawMessage(`[{"data":"*"},{"data":"-"}]`) },
		"flag in tag":   func(e node, _ []node) { e["active-tag"] = json.RawMessage(`[{"data":[null]}]`) },
	} {
		t.Run(name, func(t *testing.T) {
			f, err := (Driver{}).Parse("route", mutateISIS(t, change))
			if err == nil || len(f.Routes) != 0 {
				t.Fatalf("invalid selection accepted: %v %+v", err, f)
			}
		})
	}
}

func TestRouteSelectionStates(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want *bool
	}{{"*", boolValue(true)}, {"+", boolValue(true)}, {"-", boolValue(false)}, {"", nil}} {
		t.Run(tc.tag, func(t *testing.T) {
			data := mutateISIS(t, func(e node, h []node) {
				b, _ := json.Marshal([]map[string]string{{"data": tc.tag}})
				e["active-tag"] = b
				h[0]["selected-next-hop"] = json.RawMessage(`[]`)
			})
			f, err := (Driver{}).Parse("route", data)
			if err != nil {
				t.Fatal(err)
			}
			r := f.Routes[0]
			if !reflect.DeepEqual(r.Selected, tc.want) || r.NextHops[0].Active != nil || r.NextHops[1].Active != nil || r.Installed != nil {
				t.Fatal("selection/unknowns", r)
			}
		})
	}
	data := mutateISIS(t, func(e node, _ []node) { delete(e, "active-tag") })
	f, err := (Driver{}).Parse("route", data)
	if err != nil || f.Routes[0].Selected != nil {
		t.Fatal(f, err)
	}
}
func boolValue(v bool) *bool { return &v }
