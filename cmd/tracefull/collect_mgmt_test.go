package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/amb1s1/tracefull/internal/collect"
)

func testManagementExecutable(t *testing.T, bin string) {
	for _, platform := range []string{"cumulus", "frr"} {
		t.Run(platform, func(t *testing.T) {
			dir := t.TempDir()
			fixtures, err := filepath.Abs("../../internal/driver/cumulus/testdata")
			if err != nil {
				t.Fatal(err)
			}
			// Synthetic trace and wrapper replay recorded device formats without SSH.
			script := `#!/bin/sh
printf ran > "$MARKER"
case "$2" in
 "ip -j -d address show") file=addresses-detailed.json ;;
 "sudo -n vtysh -c 'show ipv6 route vrf mgmt 2001:db8:2::3 json'") file=frr-route-mgmt.json ;;
 "ip -j -s link show dev eth0") file=counters.json ;;
 "sudo -n /usr/sbin/lldpctl -f json") file=lldp.json ;;
 "sudo -n vtysh -c 'show version'") file=frr-version.txt ;;
 "cat /etc/os-release") file=os-release.txt ;;
 "cat /proc/uptime") file=uptime.txt ;;
 *) exit 9 ;;
esac
exec /bin/cat "$CAPTURES/$file"
`
			for name, data := range map[string]string{"replay": script, "inventory.yaml": "devices:\n  - name: switch-a\n    address: 2001:db8:2::3\n    platform: " + platform + "\n", "trace.txt": "traceroute to 2001:db8:2::3 (2001:db8:2::3), 30 hops max\n1 2001:db8:2::3 1 ms 2 ms 3 ms\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(dir, "marker")
			args := []string{"collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--vrf", "mgmt", "--sudo", "--json", "--ssh-cmd", filepath.Join(dir, "replay") + " {address} {cmd}"}
			run := func(extra ...string) []byte {
				c := exec.Command(bin, append(append([]string{}, args...), extra...)...)
				c.Dir = dir
				c.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "CAPTURES=" + fixtures, "MARKER=" + marker}
				b, err := c.CombinedOutput()
				if err != nil {
					t.Fatalf("CLI: %v %s", err, b)
				}
				return b
			}
			if !json.Valid(run("--dry-run")) {
				t.Fatal("invalid preview")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("preview executed a command")
			}
			var r collect.Report
			if err := json.Unmarshal(run(), &r); err != nil {
				t.Fatal(err)
			}
			if r.Status != "complete" || len(r.Devices) != 1 || !reflect.DeepEqual(r.Devices[0].Interfaces, []string{"eth0"}) {
				t.Fatal(r)
			}
			want := 6
			if platform == "frr" {
				want++
			}
			if len(r.Devices[0].Commands) != want {
				t.Fatal(r.Devices[0].Commands)
			}
			for _, c := range r.Devices[0].Commands {
				if c.Status != "ok" {
					t.Fatal(c)
				}
			}
		})
	}
}
