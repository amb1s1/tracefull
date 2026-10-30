package junos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

var captures = map[string]string{"version": "version-json", "addresses": "addresses", "route": "route4-connected", "uptime": "uptime", "cpu": "cpu", "lldp": "lldp", "counters-et-0/0/0": "counters-physical", "counters-et-0/0/0.0": "counters-logical", "optics-et-0/0/0": "optics"}

func TestCaptures(t *testing.T) {
	for name, file := range captures {
		f, e := (Driver{}).Parse(name, fixture(t, file))
		if e != nil {
			t.Fatal(name, e)
		}
		switch name {
		case "version":
			if f.Version.Version != "24.4R2-S5.3" {
				t.Fatal(f)
			}
		case "addresses":
			if len(f.Addresses) == 0 {
				t.Fatal(f)
			}
			for _, a := range f.Addresses {
				if a.RoutingContext != nil {
					t.Fatal("invented context")
				}
			}
		case "route":
			if len(f.Routes) != 1 || f.Routes[0].Protocol != "Direct" || f.Routes[0].NextHops[0].Interface != "et-0/0/0.0" || f.Routes[0].Installed != nil {
				t.Fatal(f)
			}
		case "uptime":
			if *f.UptimeSeconds != 650257 {
				t.Fatal(f)
			}
		case "cpu":
			if *f.CPU.Load1 != 0.32 || *f.CPU.Load5 != 0.27 || *f.CPU.Load15 != 0.29 {
				t.Fatal(f)
			}
		case "optics-et-0/0/0":
			if len(f.Optics) != 4 || *f.Optics[0].TemperatureCelsius != 51.7 || *f.Optics[0].TXPowerDBM != 1.62 {
				t.Fatal(f)
			}
		case "counters-et-0/0/0":
			if len(f.Counters) != 1 || f.Counters[0].MTULayer != "link" || f.Counters[0].RXErrors == nil {
				t.Fatal(f)
			}
		case "counters-et-0/0/0.0":
			if f.Counters[0].RXErrors != nil {
				t.Fatal("invented logical errors")
			}
		}
	}
	f, e := (Driver{}).Parse("route", fixture(t, "route6"))
	if e != nil || len(f.Routes) != 0 {
		t.Fatal("empty route rejected", e)
	}
}
func TestPolicyAndFailures(t *testing.T) {
	d := Driver{}
	p := driver.Params{Destination: "192.0.2.1", Interfaces: []string{"et-0/0/0.0", "lo0.0"}}
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
	for _, cmd := range []string{"show route table inet6.0 192.0.2.1 detail | display json", "show interfaces et-0/0/0 extensive | display json | sh", "show version | display xml", "request system reboot", "show interfaces -bad extensive | display json"} {
		if d.Validate(cmd, p) == nil {
			t.Fatal(cmd)
		}
	}
	p.Interfaces = nil
	if _, e := d.Plan(groups, p); e != nil {
		t.Fatal(e)
	}
	p.VRF = "x"
	if d.Validate("show version | display json", p) == nil {
		t.Fatal("VRF policy")
	}
	if _, e := d.Plan(groups, p); e == nil {
		t.Fatal("VRF fallback")
	}
	if !reflect.DeepEqual(d.ExpandInterfaces([]string{"et-0/0/0.0", "et-0/0/0"}), []string{"et-0/0/0", "et-0/0/0.0"}) {
		t.Fatal("parent scope")
	}
	for name := range captures {
		for _, data := range []string{"", "null", "{}", "{", `{"rpc-error":[{}]}`} {
			if _, e := d.Parse(name, []byte(data)); e == nil {
				t.Fatal(name, data)
			}
		}
	}
	f, e := d.Parse("cpu", []byte(`{"route-engine-information":[{}]}`))
	if e != nil || f.CPU != nil {
		t.Fatal("missing master", f, e)
	}
	if _, e := d.Parse("unknown", []byte(`{}`)); e == nil {
		t.Fatal("unknown parser")
	}
}
func FuzzParse(f *testing.F) {
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

func TestMalformedNestedContainers(t *testing.T) {
	// Synthetic corruption must not become an apparently empty address snapshot.
	for _, data := range []string{
		`{"interface-information":[{"physical-interface":{}}]}`,
		`{"interface-information":[{"physical-interface":[null]}]}`,
		`{"interface-information":[{"data":42}]}`,
		`{"interface-information":[{"attributes":[]}]}`,
		`{"interface-information":[{"data":null}]}`,
	} {
		if _, err := (Driver{}).Parse("addresses", []byte(data)); err == nil {
			t.Fatal(data)
		}
	}
}

func TestRouteGolden(t *testing.T) {
	facts, err := (Driver{}).Parse("route", fixture(t, "route4-connected"))
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
