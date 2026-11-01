package nxos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

var captures = map[string]string{"version": "version-json", "addresses4": "addresses4-all", "addresses6": "addresses6-all", "route": "route6", "cpu": "cpu", "lldp": "lldp", "counters-mgmt0": "counters-mgmt", "counters-Ethernet1/1": "counters-uplink", "optics-Ethernet1/1": "optics-uplink"}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", name+".json"))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestCaptures(t *testing.T) {
	for name, file := range captures {
		t.Run(name, func(t *testing.T) {
			f, e := (Driver{}).Parse(name, fixture(t, file))
			if e != nil {
				t.Fatal(e)
			}
			switch name {
			case "version":
				if f.Version.Version != "10.5(6)" || *f.UptimeSeconds != 43828 {
					t.Fatal(f)
				}
			case "addresses4":
				if len(f.Addresses) != 1 || *f.Addresses[0].RoutingContext != "management" || f.Addresses[0].PrefixLength != nil {
					t.Fatal(f)
				}
			case "addresses6":
				if len(f.Addresses) != 2 || f.Addresses[1].Address != "fe80::c" {
					t.Fatal(f)
				}
			case "route":
				r := f.Routes[0]
				if r.Prefix != "::/0" || r.Protocol != "static" || !*r.Selected || r.Installed != nil || len(r.NextHops) != 1 || r.NextHops[0].Active != nil || r.NextHops[0].Installed != nil {
					t.Fatal(r)
				}
			case "cpu":
				if *f.CPU.Utilization != 16.75 || *f.CPU.Load1 != 1.25 || f.CPU.Utilization1 != nil {
					t.Fatal(f)
				}
			case "lldp":
				if len(f.Neighbors) != 2 || f.Neighbors[1].Interface != "Ethernet1/1" {
					t.Fatal(f)
				}
			case "counters-mgmt0":
				if *f.Counters[0].SpeedBPS != 1000000000 || f.Counters[0].RXErrors != nil {
					t.Fatal(f)
				}
			case "counters-Ethernet1/1":
				if *f.Counters[0].SpeedBPS != 10000000000 || *f.Counters[0].MTUBytes != 9216 || *f.Counters[0].RXErrors != 0 || f.Counters[0].MTULayer != "" {
					t.Fatal(f)
				}
			case "optics-Ethernet1/1":
				o := f.Optics[0]
				if o.Lane != "" || *o.TemperatureCelsius != 40.74 || *o.RXPowerDBM != -2 || *o.TXPowerDBM != -2.85 {
					t.Fatal(o)
				}
			}
		})
	}
	f, e := (Driver{}).Parse("route", fixture(t, "route4"))
	if e != nil || len(f.Routes) != 0 {
		t.Fatal(f, e)
	}
}
func TestPolicy(t *testing.T) {
	d := Driver{}
	groups := []string{"ingress", "route", "version", "cpu", "counters", "lldp", "optics", "reverse"}
	for _, dst := range []string{"192.0.2.1", "2001:db8::1"} {
		for _, ifaces := range [][]string{nil, {"mgmt0", "Ethernet1/1"}} {
			p := driver.Params{Destination: dst, Interfaces: ifaces}
			commands, e := d.Plan(groups, p)
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range commands {
				if c.Status == "planned" {
					if e := d.Validate(c.Text, p); e != nil {
						t.Fatal(c, e)
					}
				}
			}
		}
	}
	for _, cmd := range []string{"show version", "show version | json | sh", "show interface Ethernet1;reload | json", "show ip route ::1 | json", "show ipv6 route fe80::1%eth0 | json", "show interface mgmt0 transceiver details | json", "configure terminal", "show interface x | json; id"} {
		if d.Validate(cmd, driver.Params{}) == nil {
			t.Fatal(cmd)
		}
	}
	for _, p := range []driver.Params{{VRF: "management"}, {Destination: "bad"}, {Destination: "1.1.1.1", Interfaces: []string{"x;id"}}} {
		if _, e := d.Plan(groups, p); e == nil {
			t.Fatal(p)
		}
	}
	if d.Validate("show version | json", driver.Params{VRF: "management"}) == nil {
		t.Fatal("context fallback")
	}
}
func TestMalformedAndUnknown(t *testing.T) {
	for name := range captures {
		for _, data := range []string{"", "null", "{}", "[]", "{", strings.Repeat(" ", 1<<20+1)} {
			if _, e := (Driver{}).Parse(name, []byte(data)); e == nil {
				t.Fatal(name, data[:min(len(data), 30)])
			}
		}
	}
	if _, e := (Driver{}).Parse("unknown", []byte(`{}`)); e == nil {
		t.Fatal("unknown observation")
	}
	// Synthetic mutations test incorrect types at every captured leaf, including
	// numeric overflow, null containers and array rows without changing real fixtures.
	for name, file := range captures {
		var root any
		if e := json.Unmarshal(fixture(t, file), &root); e != nil {
			t.Fatal(e)
		}
		var walk func(any)
		walk = func(v any) {
			switch obj := v.(type) {
			case map[string]any:
				for k, old := range obj {
					if _, ok := old.(string); ok {
						obj[k] = map[string]any{"invalid": true}
						b, _ := json.Marshal(root)
						f, e := (Driver{}).Parse(name, b)
						if e != nil && !reflect.DeepEqual(f, driver.Empty()) {
							t.Fatal("partial facts on failure", name, k)
						}
						obj[k] = old
					}
					walk(old)
				}
			case []any:
				for _, x := range obj {
					walk(x)
				}
			}
		}
		walk(root)
	}
	for _, tc := range []struct{ name, data string }{
		{"route", `{"TABLE_vrf":{"ROW_vrf":null}}`},
		{"route", `{"TABLE_vrf":{"ROW_vrf":[null]}}`},
		{"route", `{"TABLE_vrf":{"ROW_vrf":{"vrf-name-out":"default"}}}`},
		{"addresses6", `{"TABLE_intf":{"ROW_intf":{"intf-name":"mgmt0","vrf-name-out":"management","prefix":"bad"}}}`},
		{"cpu", `{"cpu_state_idle":"101"}`}, {"cpu", `{"cpu_state_idle":"NaN"}`}, {"cpu", `{"load_avg_1min":"-1"}`},
		{"counters-x", `{"TABLE_interface":{"ROW_interface":{"interface":"x","eth_speed":"fast"}}}`},
		{"counters-x", `{"TABLE_interface":{"ROW_interface":{"interface":"x","eth_mtu":"-1"}}}`},
	} {
		if _, e := (Driver{}).Parse(tc.name, []byte(tc.data)); e == nil {
			t.Fatal(tc)
		}
	}
	// Optional counter fields remain unknown, and actual array evidence is retained.
	f, e := (Driver{}).Parse("counters-x", []byte(`{"TABLE_interface":{"ROW_interface":[{"interface":"x"}]}}`))
	if e != nil || f.Counters[0].RXErrors != nil || f.Counters[0].Evidence != "json:/TABLE_interface/ROW_interface/0" {
		t.Fatal(f, e)
	}
}
func FuzzParse(f *testing.F) {
	for name, file := range captures {
		f.Add(name, fixture(f, file))
	}
	f.Add("route", fixture(f, "route4"))
	f.Fuzz(func(t *testing.T, name string, b []byte) {
		a, e := (Driver{}).Parse(name, b)
		c, e2 := (Driver{}).Parse(name, b)
		if (e == nil) != (e2 == nil) || !reflect.DeepEqual(a, c) {
			t.Fatal("nondeterministic")
		}
		if e == nil {
			if _, e := json.Marshal(a); e != nil {
				t.Fatal(e)
			}
		}
	})
}

func TestRouteSelectionVariants(t *testing.T) {
	// Synthetic variations of the real static route test explicit false/unknown
	// state and same-context selection without claiming another device capture.
	original := string(fixture(t, "route6"))
	for _, tc := range []struct {
		value       string
		known, want bool
	}{{"true", true, true}, {"false", true, false}, {"", false, false}} {
		data := strings.ReplaceAll(original, `"management"`, `"default"`)
		data = strings.Replace(data, `"ubest": "true"`, `"ubest": "`+tc.value+`"`, 1)
		f, e := (Driver{}).Parse("route", []byte(data))
		if e != nil {
			t.Fatal(e)
		}
		r := f.Routes[0]
		if (r.Selected != nil) != tc.known || (r.NextHops[0].Active != nil) != tc.known || tc.known && (*r.Selected != tc.want || *r.NextHops[0].Active != tc.want) {
			t.Fatal(r)
		}
	}
	for _, change := range [][2]string{{`"ubest": "true"`, `"ubest": "yes"`}, {`"ipprefix": "0::/0"`, `"ipprefix": "bad"`}, {`"ipv6prefix": "0::/0"`, `"ipv6prefix": "2001:db8::/32"`}, {`"ipnexthop": "2001:db8:1::3"`, `"ipnexthop": "bad"`}, {`"ipv6nexthop": "2001:db8:1::3"`, `"ipv6nexthop": "::1"`}, {`"addrf": "ipv6"`, `"addrf": "ipx"`}} {
		if _, e := (Driver{}).Parse("route", []byte(strings.Replace(original, change[0], change[1], 1))); e == nil {
			t.Fatal(change)
		}
	}
}
