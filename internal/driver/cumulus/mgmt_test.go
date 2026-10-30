package cumulus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestManagementCaptures(t *testing.T) {
	d := Driver{}
	f, err := d.Parse("addresses-detailed", capture(t, "addresses-detailed.json"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, a := range f.Addresses {
		want := "default"
		if a.Interface == "eth0" || a.Interface == "mgmt" {
			want = "mgmt"
		}
		if a.Interface == "tenant-a" {
			want = "tenant-a"
		}
		if a.RoutingContext == nil || *a.RoutingContext != want || !strings.HasPrefix(a.Evidence, "json:/") {
			t.Fatalf("context: %+v", a)
		}
		seen[a.Interface] = true
	}
	for _, name := range []string{"lo", "eth0", "mgmt", "tenant-a"} {
		if !seen[name] {
			t.Fatal("missing captured interface", name)
		}
	}
	for _, file := range []string{"frr-route-mgmt.json", "frr-route-mgmt4.json"} {
		f, err = d.Parse("route-mgmt", capture(t, file))
		if err != nil || len(f.Routes) != 1 || f.Routes[0].Context != "mgmt" {
			t.Fatal(f, err)
		}
		r := f.Routes[0]
		if r.Selected == nil || !*r.Selected || r.Installed == nil || !*r.Installed {
			t.Fatal(r)
		}
		if file == "frr-route-mgmt.json" && (r.Prefix != "2001:db8:2::3/128" || r.Protocol != "local" || len(r.NextHops) != 1 || r.NextHops[0].Interface != "eth0" || r.NextHops[0].Address != "") {
			t.Fatal(r)
		}
		// The IPv4 capture is an unreachable default route. No egress was observed.
		if file == "frr-route-mgmt4.json" && (r.Prefix != "0.0.0.0/0" || r.NextHops[0].Interface != "" || r.NextHops[0].Address != "") {
			t.Fatal(r)
		}
	}
}

func TestDetailedContextUncertainty(t *testing.T) {
	// Synthetic mutations of the captured metadata exercise incomplete/conflicting snapshots.
	for _, tc := range []struct {
		name   string
		mutate func([]link)
		iface  string
	}{
		{"missing parent", func(r []link) { r[2].Name = "missing" }, "eth0"},
		{"wrong parent type", func(r []link) { r[2].LinkInfo.Kind = "bridge" }, "eth0"},
		{"conflicting member table", func(r []link) { v := uint32(999); r[1].LinkInfo.Data.Table = &v }, "eth0"},
		{"missing member type", func(r []link) { r[1].LinkInfo.SlaveKind = "" }, "eth0"},
		{"missing member table", func(r []link) { r[1].LinkInfo.SlaveData.Table = nil }, "eth0"},
		{"table mismatch", func(r []link) { v := uint32(999); r[1].LinkInfo.SlaveData.Table = &v }, "eth0"},
		{"missing parent table", func(r []link) { r[2].LinkInfo.Data.Table = nil }, "mgmt"},
		{"zero parent table", func(r []link) { v := uint32(0); r[2].LinkInfo.Data.Table = &v }, "mgmt"},
		{"parent has master", func(r []link) { r[2].Master = "other" }, "mgmt"},
		{"orphan slave", func(r []link) { r[1].Master = "" }, "eth0"},
		{"missing flags", func(r []link) { r[0].Flags = nil }, "lo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := links(capture(t, "addresses-detailed.json"))
			if err != nil {
				t.Fatal(err)
			}
			// Arrange by name so fixture ordering is not part of the mutation contract.
			ordered := []link{}
			for _, name := range []string{"lo", "eth0", "mgmt"} {
				for _, row := range rows {
					if row.Name == name {
						ordered = append(ordered, row)
					}
				}
			}
			rows = ordered
			tc.mutate(rows)
			b, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			f, err := (Driver{}).Parse("addresses-detailed", b)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, a := range f.Addresses {
				if a.Interface == tc.iface {
					found = true
					if a.RoutingContext != nil {
						t.Fatal(a)
					}
				}
			}
			if !found {
				t.Fatal("missing test address")
			}
		})
	}
	for _, input := range []string{`[{"ifname":"a"},{"ifname":"a"}]`, `[{"ifname":"a","linkinfo":{"info_data":{"table":-1}}}]`} {
		if _, err := (Driver{}).Parse("addresses-detailed", []byte(input)); err == nil {
			t.Fatal("accepted malformed snapshot")
		}
	}
}

func TestManagementRouteRejectsContextFallback(t *testing.T) {
	d := Driver{}
	for _, context := range []string{"default", "", "tenant-a"} {
		b := strings.ReplaceAll(string(capture(t, "frr-route-mgmt.json")), `"vrfName": "mgmt"`, `"vrfName": "`+context+`"`)
		f, err := d.Parse("route-mgmt", []byte(b))
		if err == nil || len(f.Routes) != 0 {
			t.Fatal("accepted context fallback", f, err)
		}
	}
	f, err := d.Parse("route-mgmt", []byte(`{}`))
	if err != nil || len(f.Routes) != 0 {
		t.Fatal("invented absent route", f, err)
	}
}

func TestManagementCommandPolicy(t *testing.T) {
	d := Driver{}
	for _, sudo := range []bool{false, true} {
		for _, dest := range []string{"2001:db8:2::3", "127.0.0.1"} {
			p := driver.Params{VRF: "mgmt", Sudo: sudo, Destination: dest}
			commands, err := d.Plan([]string{"ingress", "route", "counters", "lldp", "version"}, p)
			if err != nil {
				t.Fatal(err)
			}
			if commands[0].Name != "addresses-detailed" || commands[1].Name != "route-mgmt" || !strings.Contains(commands[1].Text, " route vrf mgmt "+dest+" json'") {
				t.Fatal(commands)
			}
			for _, c := range commands {
				if c.Status == "planned" && d.Validate(c.Text, p) != nil {
					t.Fatal(c)
				}
			}
			other := p
			other.VRF = "default"
			if d.Validate(commands[1].Text, other) == nil || d.Validate(commands[0].Text, other) == nil {
				t.Fatal("management command accepted by default policy")
			}
			for _, cmd := range []string{"ip -j address show", "vtysh -c 'show ipv6 route 2001:db8::1 json'", "sudo -n vtysh -c 'show ipv6 route 2001:db8::1 json'", "vtysh -c 'show ipv6 route vrf mgmt 192.0.2.1 json'", "vtysh -c 'show ipv6 route vrf mgmt fe80::1%eth0 json'", "vtysh -c 'show ipv6 route vrf mgmt 2001:db8::1 json'; reboot"} {
				if d.Validate(cmd, p) == nil {
					t.Fatal("unsafe/unscoped command", cmd)
				}
			}
		}
	}
}
