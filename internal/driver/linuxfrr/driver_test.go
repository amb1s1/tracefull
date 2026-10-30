package linuxfrr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/driver/cumulus"
)

func capture(t testing.TB, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "cumulus", "testdata", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestSharedCaptures(t *testing.T) {
	d := Driver{}
	for name, file := range map[string]string{"version": "os-release.txt", "addresses": "addresses.json", "route": "frr-route.json", "counters-swp49": "counters.json", "lldp": "lldp.json", "uptime": "uptime.txt", "cpu": "loadavg.txt"} {
		data := capture(t, file)
		got, e := d.Parse(name, data)
		if e != nil {
			t.Fatal(e)
		}
		want, e := (cumulus.Driver{}).Parse(name, data)
		if e != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(name, "shared facts changed", e)
		}
	}
	got, e := d.Parse("frr-version", capture(t, "frr-version.txt"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile("testdata/frr-version.golden.json")
	if e != nil {
		t.Fatal(e)
	}
	var want driver.Version
	if e = json.Unmarshal(b, &want); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(got.Version, &want) {
		t.Fatal(got.Version, want)
	}
	if d.Platform() != "frr" {
		t.Fatal("wrong platform")
	}
}
func TestPlanAndPolicy(t *testing.T) {
	d := Driver{}
	for _, sudo := range []bool{false, true} {
		p := driver.Params{Destination: "2001:db8::1", Interfaces: []string{"eth0"}, Sudo: sudo}
		cmds, e := d.Plan([]string{"ingress", "route", "version", "cpu", "counters", "lldp", "optics", "reverse"}, p)
		if e != nil {
			t.Fatal(e)
		}
		versions := 0
		for _, c := range cmds {
			if c.Status == "planned" {
				if e = d.Validate(c.Text, p); e != nil {
					t.Fatal(c, e)
				}
			}
			if c.Name == "frr-version" {
				versions++
				if strings.HasPrefix(c.Text, "sudo -n ") != sudo {
					t.Fatal("implicit privilege")
				}
			}
		}
		if versions != 1 {
			t.Fatal(versions)
		}
		for _, cmd := range []string{"vtysh -c 'configure terminal'", "vtysh -c 'show version; write'", "vtysh -c 'show version' | sh", "sudo vtysh -c 'show version'"} {
			if d.Validate(cmd, p) == nil {
				t.Fatal(cmd)
			}
		}
	}
	if _, e := d.Plan([]string{"version"}, driver.Params{VRF: "tenant"}); e == nil {
		t.Fatal("VRF fallback")
	}
	if d.Validate("vtysh -c 'show version'", driver.Params{VRF: "tenant"}) == nil {
		t.Fatal("VRF policy")
	}
	if _, e := d.Plan([]string{"cpu"}, driver.Params{}); e != nil {
		t.Fatal(e)
	}
}
func TestVersionFailuresAndBranding(t *testing.T) {
	for _, name := range []string{"version", "frr-version"} {
		for _, data := range []string{"", "garbage", strings.Repeat("x", 1<<20+1)} {
			if _, e := (Driver{}).Parse(name, []byte(data)); e == nil {
				t.Fatal(name)
			}
		}
	}
	for _, s := range []string{"ID=linux\nNAME=\"bad", "ID=linux\nNAME='unvalidated'", "ID=linux\nNAME=\"bad\\n\"", "ID=BAD\nNAME=Linux"} {
		if _, e := (Driver{}).Parse("version", []byte(s)); e == nil {
			t.Fatal(s)
		}
	}
	// Synthetic label variation proves no brand dependency, not another distro capture.
	f, e := (Driver{}).Parse("version", []byte("ID=linux\nNAME=Linux\nIGNORED=field\n# comment\n"))
	if e != nil || f.Version.OS != "Linux" || f.Version.Version != "" {
		t.Fatal(f, e)
	}
}
func FuzzParse(f *testing.F) {
	f.Add("frr-version", capture(f, "frr-version.txt"))
	f.Add("version", capture(f, "os-release.txt"))
	f.Add("route", capture(f, "frr-route.json"))
	f.Add("route-mgmt", capture(f, "frr-route-mgmt.json"))
	f.Add("addresses-detailed", capture(f, "addresses-detailed.json"))
	f.Fuzz(func(t *testing.T, name string, data []byte) {
		a, e := (Driver{}).Parse(name, data)
		b, e2 := (Driver{}).Parse(name, data)
		if (e == nil) != (e2 == nil) || !reflect.DeepEqual(a, b) {
			t.Fatal("nondeterministic")
		}
		if e == nil {
			if _, e = json.Marshal(a); e != nil {
				t.Fatal(e)
			}
		}
	})
}
