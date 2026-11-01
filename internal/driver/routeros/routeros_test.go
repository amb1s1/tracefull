package routeros

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

var captures = map[string]string{"version": "resource.txt", "cpu": "resource.txt", "addresses4": "addresses4.json", "addresses6": "addresses6.json", "route-1.1.1.1": "route4.json", "route-2606:4700:4700::1111": "route6.json", "lldp": "neighbors.json", "counters-sfp28-1": "counters.json", "optics-sfp28-1": "optics.txt"}

func fixture(t testing.TB, file string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", file))
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
				if f.Version.Version != "7.24.5 (stable)" || *f.UptimeSeconds != 1681207 {
					t.Fatal(f)
				}
			case "cpu":
				if *f.CPU.Utilization != 0 || f.CPU.Load1 != nil {
					t.Fatal(f)
				}
			case "addresses4":
				if len(f.Addresses) != 3 || f.Addresses[0].Interface != "sfp28-1" || *f.Addresses[0].PrefixLength != 30 || *f.Addresses[0].RoutingContext != "default" {
					t.Fatal(f)
				}
			case "addresses6":
				if len(f.Addresses) != 9 || f.Addresses[1].Interface != "wg-peer1" || *f.Addresses[1].PrefixLength != 125 {
					t.Fatal(f)
				}
			case "route-1.1.1.1":
				r := f.Routes[0]
				if len(f.Routes) != 1 || r.Prefix != "0.0.0.0/0" || r.Protocol != "static" || !*r.Selected || r.Installed != nil || r.NextHops[0].Interface != "sfp28-1" {
					t.Fatal(f)
				}
			case "route-2606:4700:4700::1111":
				if len(f.Routes) != 2 || f.Routes[0].Selected != nil || f.Routes[0].NextHops[0].Active != nil || !*f.Routes[1].Selected || f.Routes[1].NextHops[0].Interface != "wg-peer1" || f.Routes[1].Installed != nil {
					t.Fatal(f)
				}
			case "lldp":
				if len(f.Neighbors) != 6 || f.Neighbors[0].Interface != "sfp28-2" || f.Neighbors[1].Interface != "bridge1" || len(f.Neighbors[0].ManagementAddresses) != 1 {
					t.Fatal(f)
				}
			case "counters-sfp28-1":
				c := f.Counters[0]
				if *c.MTUBytes != 1500 || *c.TXDrops != 109 || c.RXErrors != nil || c.RXDrops != nil || c.SpeedBPS != nil || c.MTULayer != "" {
					t.Fatal(c)
				}
			case "optics-sfp28-1":
				o := f.Optics[0]
				if o.Lane != "" || *o.TemperatureCelsius != 36 || *o.TXPowerDBM != -6.495 || *o.RXPowerDBM != -6.817 {
					t.Fatal(o)
				}
			}
		})
	}
}
func TestPolicy(t *testing.T) {
	d := Driver{}
	groups := []string{"ingress", "route", "version", "cpu", "counters", "optics", "lldp", "reverse"}
	for _, dst := range []string{"192.0.2.1", "2001:db8::1"} {
		for _, ifaces := range [][]string{nil, {"sfp28-1", "wg-peer1"}} {
			p := driver.Params{Destination: dst, Interfaces: ifaces}
			cmds, e := d.Plan(groups, p)
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range cmds {
				if c.Status == "planned" {
					if e := d.Validate(c.Text, p); e != nil {
						t.Fatal(c, e)
					}
				}
			}
		}
	}
	for _, cmd := range []string{"/system reset-configuration", "/system resource print; /user set admin password=x", serialized("/ip route print detail as-value where ::1 in dst-address and routing-table=main"), serialized("/ipv6 route print detail as-value where fe80::1%eth0 in dst-address and routing-table=main"), serialized("/ip address print as-value file=output"), counterCommand("x];/system reboot;["), strings.Replace(counterCommand("sfp28-1"), "stats-detail", "stats-detail file=out", 1), strings.Replace(counterCommand("sfp28-1"), "name=sfp28-1])", "name=sfp28-2])", 1), "/interface ethernet monitor wg-peer1 once", "/interface ethernet monitor ether1"} {
		if d.Validate(cmd, driver.Params{}) == nil {
			t.Fatal(cmd)
		}
	}
	for _, p := range []driver.Params{{VRF: "management"}, {Destination: "bad"}, {Destination: "1.1.1.1", Interfaces: []string{"x;id"}}, {Destination: "1.1.1.1", Interfaces: []string{"x/y"}}} {
		if _, e := d.Plan(groups, p); e == nil {
			t.Fatal(p)
		}
	}
	if d.Validate("/system resource print", driver.Params{VRF: "management"}) == nil {
		t.Fatal("context fallback")
	}
}
func TestFailures(t *testing.T) {
	for name := range captures {
		for _, b := range []string{"", "null", "{}", "[null]", "garbage", strings.Repeat(" ", 1<<20+1)} {
			if _, e := (Driver{}).Parse(name, []byte(b)); e == nil {
				t.Fatal(name, b[:min(len(b), 30)])
			}
		}
	}
	cases := []struct{ name, file, old, new string }{
		{"version", "resource.txt", "2w5d11h7s", "99999999999999999999999w"}, {"version", "resource.txt", "2w5d11h7s", "yesterday"},
		{"cpu", "resource.txt", "0%", "101%"}, {"cpu", "resource.txt", "0%", "NaN%"},
		{"optics-sfp28-1", "optics.txt", "36C", "NaNC"}, {"optics-sfp28-1", "optics.txt", "dBm", "mW"}, {"optics-sfp28-1", "optics.txt", "sfp28-1", "sfp28-2"},
		{"route-1.1.1.1", "route4.json", "\"main\"", "\"tenant\""}, {"route-1.1.1.1", "route4.json", "0.0.0.0/0", "192.0.2.0/24"}, {"route-1.1.1.1", "route4.json", "%sfp28-1", "%bad/interface"}, {"route-1.1.1.1", "route4.json", "\"inactive\": false", "\"inactive\": true"},
		{"counters-sfp28-1", "counters.json", "1500", "-1"}, {"counters-sfp28-1", "counters.json", `"name": "sfp28-1"`, `"name": "sfp28-2"`},
		{"addresses4", "addresses4.json", "/30", "/999"}, {"addresses4", "addresses4.json", "\"main\"", "\"\""},
	}
	for _, tc := range cases {
		b := string(fixture(t, tc.file))
		if !strings.Contains(b, tc.old) {
			t.Fatal("mutation did not apply", tc)
		}
		b = strings.Replace(b, tc.old, tc.new, 1)
		f, e := (Driver{}).Parse(tc.name, []byte(b))
		if e == nil || !reflect.DeepEqual(f, driver.Empty()) {
			t.Fatal(tc, f, e)
		}
	}
	if _, e := (Driver{}).Parse("unknown", []byte("x")); e == nil {
		t.Fatal("unknown")
	}
	if _, e := (Driver{}).Parse("route-bad", []byte(`[]`)); e == nil {
		t.Fatal("bad destination")
	}
}
func TestMostSpecificAndUnknown(t *testing.T) {
	// Synthetic prefix variants exercise lookup semantics without inventing captures.
	var rows []map[string]any
	if e := json.Unmarshal(fixture(t, "route4.json"), &rows); e != nil {
		t.Fatal(e)
	}
	more := map[string]any{}
	for k, v := range rows[0] {
		more[k] = v
	}
	more["dst-address"] = "1.1.1.0/24"
	delete(more, "active")
	rows = append(rows, more)
	b, _ := json.Marshal(rows)
	f, e := (Driver{}).Parse("route-1.1.1.1", b)
	if e != nil || len(f.Routes) != 1 || f.Routes[0].Prefix != "1.1.1.0/24" || f.Routes[0].Selected != nil || f.Routes[0].NextHops[0].Active != nil {
		t.Fatal(f, e)
	}
	for _, name := range []string{"addresses4", "addresses6", "route-1.1.1.1", "lldp"} {
		if _, e := (Driver{}).Parse(name, []byte(`[]`)); e != nil {
			t.Fatal(name, e)
		}
	}
	// Disabled addresses are still in raw evidence but not asserted as owners.
	f, e = (Driver{}).Parse("addresses4", []byte(`[{"address":"192.0.2.1/24","interface":"ether1","vrf":"main","disabled":true}]`))
	if e != nil || len(f.Addresses) != 0 {
		t.Fatal(f, e)
	}
}
func FuzzParse(f *testing.F) {
	for name, file := range captures {
		f.Add(name, fixture(f, file))
	}
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

func TestOptionalStatesAndRejectedShapes(t *testing.T) {
	for _, tc := range []struct{ name, data string }{
		{"route-1.1.1.1", `[{"dst-address":"0.0.0.0/0","routing-table":"main","static":true,"bgp":true}]`},
		{"route-1.1.1.1", `[{"dst-address":"0.0.0.0/0","routing-table":"main","static":true,"gateway":"bad"}]`},
		{"route-1.1.1.1", `[{"dst-address":"0.0.0.0/0","routing-table":"main","gateway":"192.0.2.1"}]`},
		{"lldp", `[{"discovered-by":["lldp"],"interface":[]}]`},
		{"lldp", `[{"discovered-by":["lldp"],"interface":["ether1"],"address":"bad"}]`},
		{"lldp", `[{"discovered-by":["lldp"],"interface":["bad/interface"]}]`},
		{"optics-sfp28-1", "name: sfp28-1\nstatus: link-ok\nsfp-module-present: no\nsfp-temperature: 30C\n"},
		{"optics-sfp28-1", "name: sfp28-1\nstatus: link-ok\nsfp-module-present: unknown\n"},
		{"version", "platform: MikroTik\nversion: 7.24.5\nversion: 7.24.5\n"},
	} {
		if _, e := (Driver{}).Parse(tc.name, []byte(tc.data)); e == nil {
			t.Fatal(tc)
		}
	}
	f, e := (Driver{}).Parse("route-1.1.1.1", []byte(`[{"dst-address":"0.0.0.0/0","routing-table":"main","static":true,"gateway":"192.0.2.1","active":false}]`))
	if e != nil || f.Routes[0].Selected == nil || *f.Routes[0].Selected || f.Routes[0].NextHops[0].Interface != "" {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("optics-sfp28-1", []byte("name: sfp28-1\nstatus: no-link\nsfp-module-present: no\n"))
	if e != nil || len(f.Optics) != 0 {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("optics-sfp28-1", []byte("name: sfp28-1\nstatus: no-link\nsfp-module-present: yes\nsfp-temperature: N/A\n"))
	if e != nil || f.Optics[0].TemperatureCelsius != nil {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("version", []byte("platform: MikroTik\nversion: 7.24.5\n"))
	if e != nil || f.UptimeSeconds != nil {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("lldp", []byte(`[{"discovered-by":["mndp"]}]`))
	if e != nil || len(f.Neighbors) != 0 {
		t.Fatal(f, e)
	}
}
