package eos

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", name+".json"))
	if e != nil {
		t.Fatal(e)
	}
	return b
}

var captures = map[string]string{"cpu": "cpu", "version": "version-json", "addresses4": "addresses4", "addresses6": "addresses6", "route": "route6", "lldp": "lldp", "counters-Ethernet49": "counters-uplink", "counters-Vlan2602": "counters-vlan", "optics-Ethernet49": "optics-uplink", "optics-Ethernet1": "optics"}

func TestCaptures(t *testing.T) {
	d := Driver{}
	for name, file := range captures {
		f, e := d.Parse(name, fixture(t, file))
		if e != nil {
			t.Fatal(name, e)
		}
		if _, e = json.Marshal(f); e != nil {
			t.Fatal(e)
		}
		switch name {
		case "cpu":
			if f.CPU.Utilization1 != nil || math.Abs(*f.CPU.Utilization-3.1) > 1e-9 || *f.CPU.Load1 != 0.73 || *f.CPU.Load5 != 0.65 || *f.CPU.Load15 != 0.6 {
				t.Fatal(f.CPU)
			}
		case "version":
			if f.Version.Version != "4.33.4M" || *f.UptimeSeconds != 16286686.67 {
				t.Fatal(f)
			}
		case "addresses4":
			if len(f.Addresses) != 1 || *f.Addresses[0].PrefixLength != 32 || *f.Addresses[0].RoutingContext != "default" {
				t.Fatal(f)
			}
		case "addresses6":
			if len(f.Addresses) != 4 {
				t.Fatal(f)
			}
		case "route":
			if len(f.Routes) != 1 || f.Routes[0].Prefix != "::/0" || !*f.Routes[0].Installed || f.Routes[0].NextHops[0].Interface != "Vlan2602" {
				t.Fatal(f)
			}
		case "counters-Ethernet49":
			if len(f.Counters) != 1 || f.Counters[0].Interface != "Ethernet49" || *f.Counters[0].SpeedBPS != 10000000000 {
				t.Fatal(f)
			}
		case "counters-Vlan2602":
			if f.Counters[0].SpeedBPS != nil {
				t.Fatal("SVI has no physical line speed", f.Counters[0])
			}
		case "lldp":
			if len(f.Neighbors) != 2 {
				t.Fatal(f)
			}
		case "optics-Ethernet49":
			o := f.Optics[0]
			if *o.TemperatureCelsius != 47.765625 || *o.RXPowerDBM != -2.3515164280658922 || *o.TXPowerDBM != -2.0134935455473055 {
				t.Fatal(o)
			}
		case "optics-Ethernet1":
			if len(f.Optics) != 0 || len(f.Notes) == 0 {
				t.Fatal("empty DOM became measurements")
			}
		}
	}
}
func TestPolicyAndMissingFields(t *testing.T) {
	d := Driver{}
	p := driver.Params{Destination: "2001:db8::1", Interfaces: []string{"Ethernet49", "Vlan2602"}}
	groups := []string{"ingress", "route", "version", "counters", "lldp", "optics", "cpu", "reverse"}
	for _, dst := range []string{"2001:db8::1", "192.0.2.1"} {
		p.Destination = dst
		commands, e := d.Plan(groups, p)
		if e != nil {
			t.Fatal(e)
		}
		for _, c := range commands {
			if c.Status == "planned" {
				if e = d.Validate(c.Text, p); e != nil {
					t.Fatal(c, e)
				}
			}
		}
	}
	for _, cmd := range []string{"show version | json | sh", "show interfaces Ethernet1;reload | json", "show ip route ::1 | json", "show ipv6 route fe80::1%eth0 | json", "show version", "configure terminal", "show interfaces Ethernet1 transceiver | json; id"} {
		if d.Validate(cmd, p) == nil {
			t.Fatal(cmd)
		}
	}
	p.Interfaces = nil
	if _, e := d.Plan(groups, p); e != nil {
		t.Fatal(e)
	}
	p.VRF = "tenant"
	if _, e := d.Plan(groups, p); e == nil {
		t.Fatal("VRF fallback")
	}
	if d.Validate("show version | json", p) == nil {
		t.Fatal("VRF policy")
	}
	for _, name := range []string{"route", "version", "lldp", "addresses4", "addresses6", "counters-x", "optics-x", "unknown"} {
		for _, data := range []string{"", "null", "{}", "{"} {
			if _, e := d.Parse(name, []byte(data)); e == nil {
				t.Fatal(name, data)
			}
		}
	}
	f, e := d.Parse("counters-x", []byte(`{"interfaces":{"Ethernet1":{}}}`))
	if e != nil || f.Counters[0].MTUBytes != nil || f.Counters[0].RXErrors != nil {
		t.Fatal(f, e)
	}
	if _, e = d.Parse("version", []byte(`{"mfgName":"Arista","version":"test","uptime":-1}`)); e == nil {
		t.Fatal("negative uptime")
	}
	for _, data := range []string{`{"interfaces":{"x":{"addresses":[{"address":"bad","subnet":"::/0"}]}}}`, `{"interfaces":{"x":{"addresses":[{"address":"::1","subnet":"bad"}]}}}`} {
		if _, e = d.Parse("addresses6", []byte(data)); e == nil {
			t.Fatal(data)
		}
	}
	if _, e = d.Parse("version", []byte(strings.Repeat(" ", 1<<20+1))); e == nil {
		t.Fatal("oversize")
	}
}
func FuzzParse(f *testing.F) {
	f.Add("route", fixture(f, "route4-empty"))
	f.Add("route", []byte(`{"vrfs":{"default":{"routes":{"::/0":{"vias":[null]}}}}}`))
	for name, file := range captures {
		f.Add(name, fixture(f, file))
	}
	f.Fuzz(func(t *testing.T, name string, data []byte) {
		a, e := (Driver{}).Parse(name, data)
		b, e2 := (Driver{}).Parse(name, data)
		if (e == nil) != (e2 == nil) || !reflect.DeepEqual(a, b) {
			t.Fatal("nondeterministic")
		}
		if e == nil {
			if _, err := json.Marshal(a); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestRouteGolden(t *testing.T) {
	facts, err := (Driver{}).Parse("route", fixture(t, "route6"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/route.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(facts.Routes)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err = json.Unmarshal(want, &a); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(got, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("route facts differ from reviewed golden:\n%s", got)
	}
}

func TestCPUUnknownAndMalformed(t *testing.T) {
	for _, data := range []string{`{"cpuInfo":{"%Cpu(s)":{"idle":101}}}`, `{"cpuInfo":{"%Cpu(s)":{"idle":-1}}}`, `{"cpuInfo":{},"timeInfo":{"loadAvg":[1,2]}}`, `{"cpuInfo":{},"timeInfo":{"loadAvg":[1,-1,3]}}`, `{"cpuInfo":[]}`} {
		if _, err := (Driver{}).Parse("cpu", []byte(data)); err == nil {
			t.Fatal(data)
		}
	}
	f, err := (Driver{}).Parse("cpu", []byte(`{"cpuInfo":{}}`))
	if err != nil || f.CPU.Utilization != nil || f.CPU.Load1 != nil {
		t.Fatal(f, err)
	}
}
