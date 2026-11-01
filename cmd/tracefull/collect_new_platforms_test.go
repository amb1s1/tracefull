package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/collect"
)

func testNewPlatformPipelines(t *testing.T, bin string) {
	ros := func(s string) string {
		return ":put [:serialize to=json options=json.no-string-conversion value=[" + s + "]]"
	}
	for _, tc := range []struct {
		platform, address, dest, iface string
		commands                       map[string]string
	}{
		{"nxos", "2001:db8:1::b", "2001:db8::1", "mgmt0", map[string]string{
			"show version | json": "version-json.json", "show system resources | json": "cpu.json", "show ip interface brief vrf all | json": "addresses4-all.json", "show ipv6 interface brief vrf all | json": "addresses6-all.json", "show ipv6 route 2001:db8::1 | json": "route6.json", "show interface mgmt0 | json": "counters-mgmt.json", "show lldp neighbors detail | json": "lldp.json",
		}},
		{"iosxe", "2001:db8:2::5", "2001:db8::1", "Vlan2602", map[string]string{
			"show version": "version.txt", "show processes cpu": "cpu.txt", "show ip interface brief": "addresses4.txt", "show ipv6 interface brief": "addresses6.txt", "show ipv6 route 2001:db8::1": "route6.txt", "show interfaces Vlan2602": "counters-vlan.txt", "show lldp neighbors detail": "lldp.txt",
		}},
		{"routeros", "192.0.2.0", "1.1.1.1", "sfp28-1", map[string]string{
			"/system resource print": "resource.txt", ros("/ip address print as-value"): "addresses4.json", ros("/ipv6 address print as-value"): "addresses6.json", ros("/ip route print detail as-value where 1.1.1.1 in dst-address and routing-table=main"): "route4.json", ros("/ip neighbor print detail as-value"): "neighbors.json",
			":put [:serialize to=json options=json.no-string-conversion value=([/interface print detail as-value where name=sfp28-1],[/interface print stats-detail as-value where name=sfp28-1])]": "counters.json",
		}},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			dir := t.TempDir()
			captures, e := filepath.Abs("../../internal/driver/" + tc.platform + "/testdata")
			if e != nil {
				t.Fatal(e)
			}
			script := "#!/bin/sh\ncase \"$2\" in\n"
			for command, file := range tc.commands {
				script += " '" + command + "') exec /bin/cat \"$CAPTURES/" + file + "\" ;;\n"
			}
			script += " *) exit 9 ;;\nesac\n"
			platform := tc.platform
			if platform == "iosxe" {
				platform = "cisco-ios-xe"
			}
			// Synthetic trace/inventory/transport; all device responses are scrubbed captures.
			files := map[string]string{"replay": script, "inventory.yaml": "devices:\n  - name: device-a\n    address: " + tc.address + "\n    platform: " + platform + "\n", "trace.txt": "traceroute to " + tc.dest + " (" + tc.dest + "), 30 hops max\n1 " + tc.address + " 1 ms\n"}
			for name, data := range files {
				if e := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); e != nil {
					t.Fatal(e)
				}
			}
			for _, format := range []string{"json", "text", "markdown"} {
				c := exec.Command(bin, "collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--collect", "ingress,route,counters,lldp,version,cpu", "--ssh-cmd", filepath.Join(dir, "replay")+" {address} {cmd}", "--format", format, "--no-live")
				c.Dir = dir
				c.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "NO_COLOR=1", "CAPTURES=" + captures}
				out, e := c.CombinedOutput()
				if e != nil {
					t.Fatalf("%s: %v %s", format, e, out)
				}
				if format == "json" {
					var r collect.Report
					if e := json.Unmarshal(out, &r); e != nil {
						t.Fatal(e)
					}
					if r.Status != "complete" || len(r.Devices) != 1 || len(r.Devices[0].Interfaces) != 1 || r.Devices[0].Interfaces[0] != tc.iface {
						t.Fatal(r)
					}
					for _, c := range r.Devices[0].Commands {
						if c.Status != "ok" || c.Facts == nil || c.Stdout == nil || c.Stderr == nil {
							t.Fatal(c)
						}
					}
				} else {
					for _, s := range []string{"device-a", "stdout:", tc.iface} {
						if !strings.Contains(string(out), s) {
							t.Fatalf("missing %s: %s", s, out)
						}
					}
				}
			}
		})
	}
}
