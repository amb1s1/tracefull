package cumulus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func capture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCapturedFacts(t *testing.T) {
	d := Driver{}
	if d.Platform() != "nvidia-cumulus" {
		t.Fatal(d.Platform())
	}
	for _, tc := range []struct{ name, file string }{{"addresses", "addresses.json"}, {"route", "frr-route.json"}, {"counters-swp51", "counters.json"}, {"lldp", "lldp.json"}, {"version", "os-release.txt"}, {"uptime", "uptime.txt"}, {"cpu", "loadavg.txt"}} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := d.Parse(tc.name, capture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "addresses":
				found := false
				for _, a := range f.Addresses {
					if a.Address == "2001:db8:2::3" {
						found = true
						if a.Interface != "eth0" || a.Master != "mgmt" || a.PrefixLength == nil || *a.PrefixLength != 64 || a.Evidence != "json:/1/addr_info/0" {
							t.Fatal(a)
						}
					}
				}
				if !found {
					t.Fatal("missing ownership")
				}
			case "route":
				var want driver.Facts
				if err := json.Unmarshal(capture(t, "route.golden.json"), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(f, want) {
					t.Fatalf("route golden differs: %+v", f)
				}
				if len(f.Routes) != 1 || len(f.Routes[0].NextHops) != 8 || f.Routes[0].Context != "default" {
					t.Fatal(f.Routes)
				}
				for _, n := range f.Routes[0].NextHops {
					if n.Interface == "" || n.Installed == nil || !*n.Installed {
						t.Fatal("lost link-local scope or FIB status")
					}
				}
			case "counters-swp51":
				if len(f.Counters) != 12 {
					t.Fatal(len(f.Counters))
				}
				for _, c := range f.Counters {
					if c.MTUBytes == nil || c.RXErrors == nil || c.SpeedBPS != nil {
						t.Fatal(c)
					}
				}
			case "lldp":
				if len(f.Neighbors) != 9 || f.Neighbors[0].Name != "peer-1" || f.Neighbors[0].PortID != "Ethernet4" {
					t.Fatal(f.Neighbors)
				}
			case "version":
				if f.Version == nil || f.Version.Version != "5.16.0" || f.Version.OS != "Cumulus Linux" {
					t.Fatal(f.Version)
				}
			case "uptime":
				if f.UptimeSeconds == nil || *f.UptimeSeconds != 15098685.20 {
					t.Fatal(f.UptimeSeconds)
				}
			case "cpu":
				if f.CPU == nil || *f.CPU.Load1 != 0.50 || *f.CPU.Load5 != 0.51 || *f.CPU.Load15 != 0.39 {
					t.Fatal(f.CPU)
				}
			}
		})
	}
}

func TestMissingAndMalformed(t *testing.T) {
	d := Driver{}
	// Synthetic cases exercise unknowns and malformed fields, not device captures.
	for _, tc := range []struct{ name, data string }{
		{"addresses", `null`}, {"addresses", `{}`}, {"addresses", `[{}]`}, {"addresses", `[{"ifname":"eth0","addr_info":[{"local":"bad"}]}]`}, {"addresses", `[{"ifname":"eth0","addr_info":[{"local":"192.0.2.1","prefixlen":33}]}]`},
		{"route", `null`}, {"route", `[]`}, {"route", `{"bad":[{}]}`}, {"route", `{"::/0":[{"nexthops":[{"ip":"bad"}]}]}`},
		{"counters-eth0", `[null]`}, {"counters-eth0", `[{"ifname":"eth0","mtu":-1}]`},
		{"lldp", `{}`}, {"lldp", `[]`}, {"lldp", `{"lldp":[{"interface":[{}]}]}`}, {"lldp", `{"lldp":[{"interface":[{"name":"eth0","chassis":[{"mgmt-ip":[{"value":"bad"}]}]}]}]}`},
		{"version", "ID=debian\n"}, {"version", "ID=cumulus-linux\nNAME=\"broken"}, {"cpu", "NaN 0 0"}, {"cpu", "-1 0 0"}, {"cpu", "Inf 0 0"}, {"cpu", "bad 0 0"}, {"cpu", ""}, {"uptime", "1"}, {"uptime", "bad 1"}, {"unknown", "{}"},
	} {
		if _, err := d.Parse(tc.name, []byte(tc.data)); err == nil {
			t.Fatalf("accepted %s %s", tc.name, tc.data)
		}
	}
	if _, err := d.Parse("cpu", []byte(strings.Repeat("x", (1<<20)+1))); err == nil {
		t.Fatal("oversized output accepted")
	}
	f, err := d.Parse("counters-eth0", []byte(`[{"ifname":"eth0"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Counters[0].RXErrors != nil || f.Counters[0].MTUBytes != nil {
		t.Fatal("unknown became zero")
	}
	f, err = d.Parse("route", []byte(`{"::/0":[{"nexthops":[{"interfaceName":"eth0"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Routes[0].Installed != nil || f.Routes[0].NextHops[0].Active != nil {
		t.Fatal("unknown became false")
	}
	for _, tc := range []struct{ name, data string }{{"addresses", `[{"ifname":"eth0","addr_info":[{"local":"192.0.2.1"}]}]`}, {"route", `{}`}, {"lldp", `{"lldp":[]}`}, {"lldp", `{"lldp":[{"interface":[{"name":"eth0"}]}]}`}, {"version", "ID=cumulus-linux\n"}} {
		if _, err := d.Parse(tc.name, []byte(tc.data)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanAndPolicy(t *testing.T) {
	d := Driver{}
	p := driver.Params{Destination: "2001:db8::1"}
	groups := []string{"ingress", "route", "counters", "lldp", "version", "cpu", "optics", "reverse"}
	plan, err := d.Plan(groups, p)
	if err != nil {
		t.Fatal(err)
	}
	deferred, unsupported := 0, 0
	for _, c := range plan {
		if c.Status == "deferred" {
			deferred++
			if c.Text != "" {
				t.Fatal("deferred argv is not concrete")
			}
		}
		if c.Status == "unsupported" {
			unsupported++
		}
	}
	if deferred != 2 || unsupported != 2 {
		t.Fatal(plan)
	}
	p.Interfaces = []string{"swp51", "eth0", "swp51"}
	p.Sudo = true
	plan, err = d.Plan(groups, p)
	if err != nil {
		t.Fatal(err)
	}
	counters := 0
	for _, c := range plan {
		if c.Status == "planned" {
			if err := d.Validate(c.Text, p); err != nil {
				t.Fatal(err)
			}
		}
		if c.Group == "counters" {
			counters++
		}
	}
	if counters != 2 {
		t.Fatal(plan)
	}
	for _, command := range []string{"show version", "sudo vtysh -c 'show version'", "sudo -n vtysh -c 'configure terminal'", "ip -j -s link show dev -x", "ip -j -s link show dev swp1;id", "sudo -n /usr/sbin/lldpctl -f json | sh", "sudo -n vtysh -c 'show ipv6 route bad json'", "sudo -n vtysh -c 'show ip route ::1 json'", "sudo -n vtysh -c 'show ipv6 route fe80::1%eth0 json'", "sudo -n vtysh -c 'show ip route 192.0.2.1 json' extra"} {
		if d.Validate(command, p) == nil {
			t.Fatalf("accepted %q", command)
		}
	}
	p.Sudo = false
	if d.Validate("sudo -n /usr/sbin/lldpctl -f json", p) == nil {
		t.Fatal("implicit privilege accepted")
	}
	p.Destination = "192.0.2.1"
	if _, err := d.Plan(groups, p); err != nil {
		t.Fatal(err)
	}
	p.Destination = "bad"
	if _, err := d.Plan(groups, p); err == nil {
		t.Fatal("bad destination accepted")
	}
	p.Destination = "192.0.2.1"
	p.Interfaces = []string{"swp1;id"}
	if _, err := d.Plan(groups, p); err == nil {
		t.Fatal("bad discovery parameter accepted")
	}
	p.Interfaces = nil
	if _, err := d.Plan([]string{"arbitrary"}, p); err == nil {
		t.Fatal("unknown group accepted")
	}
	p.VRF = "tenant-a"
	if _, err := d.Plan(groups, p); err == nil {
		t.Fatal("unsupported VRF fell back")
	}
	if d.Validate("cat /etc/os-release", p) == nil {
		t.Fatal("policy silently changed context")
	}
}

func FuzzParse(f *testing.F) {
	for name, file := range map[string]string{"addresses-detailed": "addresses-detailed.json", "route-mgmt": "frr-route-mgmt.json", "addresses": "addresses.json", "route": "frr-route.json", "counters-eth0": "counters.json", "lldp": "lldp.json", "version": "os-release.txt", "cpu": "loadavg.txt", "uptime": "uptime.txt"} {
		data, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(name, data)
		f.Add(name, []byte("{}"))
	}
	f.Fuzz(func(t *testing.T, name string, data []byte) {
		a, e1 := (Driver{}).Parse(name, data)
		b, e2 := (Driver{}).Parse(name, data)
		if (e1 == nil) != (e2 == nil) || !reflect.DeepEqual(a, b) {
			t.Fatal("nondeterministic parser")
		}
		if e1 == nil {
			if _, err := json.Marshal(a); err != nil {
				t.Fatal(err)
			}
		}
	})
}
