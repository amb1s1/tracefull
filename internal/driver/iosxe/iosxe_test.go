package iosxe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

var captures = map[string]string{"version": "version", "addresses4": "addresses4", "addresses6": "addresses6", "route": "route6", "cpu": "cpu", "lldp": "lldp", "counters-Vlan2602": "counters-vlan", "counters-TenGigabitEthernet1/1/1": "counters-uplink", "optics-TenGigabitEthernet1/1/1": "optics"}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", name+".txt"))
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
				if f.Version.Version != "17.12.03" || f.UptimeSeconds != nil {
					t.Fatal(f)
				}
			case "addresses4":
				if len(f.Addresses) != 0 {
					t.Fatal(f)
				}
			case "addresses6":
				if len(f.Addresses) != 2 || f.Addresses[1].Interface != "Vlan2602" || f.Addresses[1].RoutingContext != nil || f.Addresses[1].PrefixLength != nil {
					t.Fatal(f)
				}
			case "route":
				r := f.Routes[0]
				if r.Prefix != "::/0" || r.Protocol != "static" || r.Selected != nil || r.Installed != nil || len(r.NextHops) != 1 || r.NextHops[0].Interface != "" || r.NextHops[0].Active != nil {
					t.Fatal(r)
				}
			case "cpu":
				if *f.CPU.Utilization != 13 || *f.CPU.Utilization1 != 6 || *f.CPU.Utilization5 != 5 || f.CPU.Load1 != nil || f.CPU.Utilization15 != nil {
					t.Fatal(f)
				}
			case "lldp":
				if len(f.Neighbors) < 2 || f.Neighbors[0].Interface != "TenGigabitEthernet1/1/2" || f.Neighbors[0].PortID != "Ethernet44" {
					t.Fatal(f)
				}
			case "counters-Vlan2602":
				c := f.Counters[0]
				if *c.MTUBytes != 9198 || *c.RXDrops != 326 || c.SpeedBPS != nil || c.MTULayer != "" {
					t.Fatal(c)
				}
			case "counters-TenGigabitEthernet1/1/1":
				c := f.Counters[0]
				if *c.SpeedBPS != 10000000000 || *c.RXErrors != 0 || *c.TXDrops != 0 {
					t.Fatal(c)
				}
			case "optics-TenGigabitEthernet1/1/1":
				o := f.Optics[0]
				if o.Lane != "" || *o.TemperatureCelsius != 28.1 || *o.TXPowerDBM != -2.9 || *o.RXPowerDBM != -4.5 {
					t.Fatal(o)
				}
			}
		})
	}
	f, e := (Driver{}).Parse("route", fixture(t, "route4"))
	if e != nil || len(f.Routes) != 0 {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("route", fixture(t, "route6-connected"))
	if e != nil || len(f.Routes) != 1 || f.Routes[0].Protocol != "connected" || f.Routes[0].NextHops[0].Interface != "Vlan2602" || f.Routes[0].Installed != nil {
		t.Fatal(f, e)
	}
}
func TestPolicy(t *testing.T) {
	d := Driver{}
	groups := []string{"ingress", "route", "version", "cpu", "counters", "optics", "lldp", "reverse"}
	for _, dst := range []string{"192.0.2.1", "2001:db8::1"} {
		for _, ifaces := range [][]string{nil, {"Vlan2602", "TenGigabitEthernet1/1/1"}} {
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
	for _, cmd := range []string{"show version | json", "show interfaces Te1/1/1;reload", "show ip route ::1", "show ipv6 route fe80::1%eth0", "show interfaces Vlan1 transceiver detail", "configure terminal", "show run", "show interfaces Te1/1/1 transceiver detail | sh"} {
		if d.Validate(cmd, driver.Params{}) == nil {
			t.Fatal(cmd)
		}
	}
	for _, p := range []driver.Params{{VRF: "management"}, {Destination: "bad"}, {Destination: "1.1.1.1", Interfaces: []string{"x;id"}}} {
		if _, e := d.Plan(groups, p); e == nil {
			t.Fatal(p)
		}
	}
	if d.Validate("show version", driver.Params{VRF: "management"}) == nil {
		t.Fatal("context fallback")
	}
}
func TestFailuresAndUnknowns(t *testing.T) {
	for name := range captures {
		for _, s := range []string{"", "garbage", "% Invalid input detected", strings.Repeat(" ", 1<<20+1)} {
			if _, e := (Driver{}).Parse(name, []byte(s)); e == nil {
				t.Fatal(name)
			}
		}
	}
	if _, e := (Driver{}).Parse("unknown", []byte("x")); e == nil {
		t.Fatal("unknown")
	}
	cases := []struct{ name, text string }{
		{"route", strings.Replace(string(fixture(t, "route6")), "static", "bgp", 1)},
		{"route", string(fixture(t, "route6")) + "% Network not in table\n"},
		{"route", string(fixture(t, "route6")) + "Routing paths:\n"},
		{"route", "Routing entry for ::/0\n  Known via \"static\"\n Routing paths:\n"},
		{"route", strings.Replace(string(fixture(t, "route6")), "::/0", "1.1.1.1/32", 1)},
		{"cpu", strings.Replace(string(fixture(t, "cpu")), "13%/3%", "101%/3%", 1)},
		{"cpu", strings.Replace(string(fixture(t, "cpu")), "13%/3%", "2%/3%", 1)},
		{"counters-x", string(fixture(t, "counters-vlan")) + " MTU 1500 bytes,\n"},
		{"counters-x", strings.Replace(string(fixture(t, "counters-vlan")), "9198", "18446744073709551616", 1)},
		{"optics-TenGigabitEthernet1/1/1", strings.Replace(string(fixture(t, "optics")), "28.1", "NaN", 1)},
		{"optics-TenGigabitEthernet1/1/1", strings.Replace(string(fixture(t, "optics")), "Te1/1/1", "Te1/1/9", 1)},
		{"optics-TenGigabitEthernet1/1/1", strings.ReplaceAll(string(fixture(t, "optics")), "N/A   -", "1     -")},
		{"addresses4", "Interface IP-Address OK? Method Status Protocol\nVlan1 bad YES unset up up\n"},
		{"addresses6", "Vlan1 [up/up]\nnot-an-ip\n"},
		{"lldp", "Local Intf: Te1/1/1\n IP: bad\n"},
	}
	for _, tc := range cases {
		f, e := (Driver{}).Parse(tc.name, []byte(tc.text))
		if e == nil || !reflect.DeepEqual(f, driver.Empty()) {
			t.Fatal(tc.name, tc.text, f, e)
		}
	}
	f, e := (Driver{}).Parse("counters-x", []byte("Vlan1 is down, line protocol is down\n"))
	if e != nil || f.Counters[0].RXErrors != nil || f.Counters[0].MTUBytes != nil {
		t.Fatal(f, e)
	}
	f, e = (Driver{}).Parse("addresses4", []byte("Interface IP-Address OK? Method Status Protocol\nGi1/0/1 192.0.2.1 YES unset up up\n"))
	if e != nil || f.Addresses[0].Interface != "GigabitEthernet1/0/1" || f.Addresses[0].RoutingContext != nil {
		t.Fatal(f, e)
	}
	// Recorded CRLF and normalized LF captures are equivalent.
	for name, file := range captures {
		lf := fixture(t, file)
		a, e := (Driver{}).Parse(name, lf)
		b, e2 := (Driver{}).Parse(name, []byte(strings.ReplaceAll(string(lf), "\n", "\r\n")))
		if e != nil || e2 != nil || !reflect.DeepEqual(a, b) {
			t.Fatal(name, e, e2)
		}
	}
}
func FuzzParse(f *testing.F) {
	for name, file := range captures {
		f.Add(name, fixture(f, file))
	}
	f.Add("route", fixture(f, "route4"))
	f.Add("route", fixture(f, "route6-connected"))
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

// Synthetic corruptions must not turn incomplete or invalid observations into facts.
func TestReviewRegressions(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"counters-x", strings.Replace(string(fixture(t, "counters-uplink")), "0 input errors", "-1 input errors", 1)},
		{"lldp", "Local Intf: Te1/1/1\nChassis id: 00:00:5e:00:53:01\n"},
		{"lldp", "Total entries displayed: 1\n"},
	} {
		if _, e := (Driver{}).Parse(tc.name, []byte(tc.text)); e == nil {
			t.Errorf("accepted malformed %s: %s", tc.name, tc.text)
		}
	}
}
