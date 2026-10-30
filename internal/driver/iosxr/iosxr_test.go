package iosxr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if e != nil {
		t.Fatal(e)
	}
	return b
}

var captures = map[string]string{"version": "version", "addresses4": "addresses4", "addresses6": "addresses6", "route": "route6-connected", "cpu": "cpu", "lldp": "lldp", "counters-FourHundredGigE0/0/0/4": "counters", "optics-FourHundredGigE0/0/0/4": "optics"}

func TestCaptures(t *testing.T) {
	for name, file := range captures {
		f, e := (Driver{}).Parse(name, fixture(t, file))
		if e != nil {
			t.Fatal(name, e)
		}
		switch name {
		case "version":
			if f.Version.Version != "25.4.2" || *f.UptimeSeconds != 108360 {
				t.Fatal(f)
			}
		case "addresses4":
			if len(f.Addresses) == 0 {
				t.Fatal(f)
			}
			for _, a := range f.Addresses {
				if a.RoutingContext == nil || a.PrefixLength != nil {
					t.Fatal("wrong nullable context/prefix", a)
				}
			}
		case "addresses6":
			if len(f.Addresses) == 0 {
				t.Fatal(f)
			}
			for _, a := range f.Addresses {
				if a.RoutingContext != nil || a.PrefixLength != nil {
					t.Fatal("invented context/prefix")
				}
			}
		case "route":
			r := f.Routes[0]
			if len(f.Routes) != 1 || r.Protocol != "connected" || r.NextHops[0].Interface != "FourHundredGigE0/0/0/4" || r.Installed != nil || !*r.Selected || r.NextHops[0].Installed != nil {
				t.Fatal(f)
			}
		case "cpu":
			if f.CPU.Load1 != nil || *f.CPU.Utilization1 != 3 || *f.CPU.Utilization5 != 3 || *f.CPU.Utilization15 != 3 {
				t.Fatal(f)
			}
		case "counters-FourHundredGigE0/0/0/4":
			c := f.Counters[0]
			if *c.MTUBytes != 9216 || *c.SpeedBPS != 400000000000 || *c.RXErrors != 0 || *c.TXDrops != 0 {
				t.Fatal(c)
			}
		case "optics-FourHundredGigE0/0/0/4":
			if len(f.Optics) != 4 || *f.Optics[0].TemperatureCelsius != 39.03 || *f.Optics[0].TXPowerDBM != 2.12 || *f.Optics[0].RXPowerDBM != 0.49 {
				t.Fatal(f)
			}
		case "lldp":
			if len(f.Neighbors) != 2 || len(f.Neighbors[0].ManagementAddresses) != 1 {
				t.Fatal(f)
			}
		}
	}
	f, e := (Driver{}).Parse("route", fixture(t, "route6"))
	if e != nil || len(f.Routes) != 0 {
		t.Fatal("missing route", e)
	}
}
func TestPolicyAndFailures(t *testing.T) {
	d := Driver{}
	p := driver.Params{Destination: "192.0.2.1", Interfaces: []string{"FourHundredGigE0/0/0/4.0", "Loopback0"}}
	groups := []string{"ingress", "route", "counters", "lldp", "optics", "version", "cpu", "reverse"}
	for _, dst := range []string{"192.0.2.1", "2001:db8::1"} {
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
	for _, cmd := range []string{"show route ipv4 ::1 detail", "show route ipv6 fe80::1%eth0 detail", "show controllers optics 0/0/0/4;reload", "show interfaces -x", "show version | json", "configure terminal"} {
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
	if d.Validate("show version", p) == nil {
		t.Fatal("VRF policy")
	}
	if len(d.ExpandInterfaces([]string{"FourHundredGigE0/0/0/4.0"})) != 2 {
		t.Fatal("physical scope")
	}
	for name := range captures {
		for _, data := range []string{"", "bad", "% Invalid input detected", "% Authorization failed"} {
			if _, e := d.Parse(name, []byte(data)); e == nil {
				t.Fatal(name, data)
			}
		}
	}
	// Configurable BW differs from the physical line rate; do not turn it into speed.
	f, e := d.Parse("counters-x", []byte("x is up, line protocol is up\n MTU 1500 bytes, BW 10 Kbit\n"))
	if e != nil || f.Counters[0].SpeedBPS != nil || f.Counters[0].RXErrors != nil {
		t.Fatal(f, e)
	}
	if _, e = d.Parse("cpu", []byte("CPU utilization for one minute: 101%; five minutes: 3%; fifteen minutes: 3%")); e == nil {
		t.Fatal("invalid CPU")
	}
	if _, e = d.Parse("unknown", []byte("data")); e == nil {
		t.Fatal("unknown parser")
	}
	if _, e = d.Parse("version", []byte(strings.Repeat("x", 1<<20+1))); e == nil {
		t.Fatal("oversized output")
	}
}
func FuzzParse(f *testing.F) {
	for _, file := range isisCaptures {
		f.Add("route", fixture(f, file))
	}
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

func TestUnvalidatedDynamicRoute(t *testing.T) {
	data := strings.ReplaceAll(string(fixture(t, "route6-connected")), `Known via "connected"`, `Known via "bgp"`)
	if _, err := (Driver{}).Parse("route", []byte(data)); err == nil {
		t.Fatal("unvalidated dynamic route reported complete")
	}
}

func TestRouteGolden(t *testing.T) {
	facts, err := (Driver{}).Parse("route", fixture(t, "route6-connected"))
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

func TestCaptureLineEndings(t *testing.T) {
	for name, file := range captures {
		data := fixture(t, file)
		lf, err := (Driver{}).Parse(name, data)
		if err != nil {
			t.Fatal(err)
		}
		crlf, err := (Driver{}).Parse(name, []byte(strings.ReplaceAll(string(data), "\n", "\r\n")))
		if err != nil || !reflect.DeepEqual(lf, crlf) {
			t.Fatal(name, "line-ending mismatch", err)
		}
	}
}
