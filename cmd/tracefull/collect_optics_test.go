package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/amb1s1/tracefull/internal/collect"
)

func testOpticsExecutable(t *testing.T, bin string) {
	for _, platform := range []string{"cumulus", "frr"} {
		t.Run(platform, func(t *testing.T) {
			dir := t.TempDir()
			fixtures, err := filepath.Abs("../../internal/driver/cumulus/testdata")
			if err != nil {
				t.Fatal(err)
			}
			// Synthetic transport: reuse one real OSFP capture to exercise interface attribution.
			script := `#!/bin/sh
printf ran > "$MARKER"
case "$2" in
 "ip -j address show") file=addresses.json ;;
 "sudo -n vtysh -c 'show ipv6 route 2001:db8::ffff json'") file=frr-route.json ;;
 "sudo -n ethtool -m eth0") printf 'no module data\n' >&2; exit 1 ;;
 "sudo -n ethtool -m swp"*) file=optics-osfp.txt ;;
 *) exit 9 ;;
esac
exec /bin/cat "$CAPTURES/$file"
`
			for name, data := range map[string]string{"replay": script, "inventory.yaml": "devices:\n  - name: switch-a\n    address: 2001:db8:2::3\n    platform: " + platform + "\n", "trace.txt": "traceroute to 2001:db8::ffff (2001:db8::ffff), 30 hops max\n1 2001:db8:2::3 1 ms 2 ms 3 ms\n"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
					t.Fatal(err)
				}
			}
			marker := filepath.Join(dir, "marker")
			args := []string{"collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--collect", "optics", "--sudo", "--json", "--ssh-cmd", filepath.Join(dir, "replay") + " {address} {cmd}"}
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
				t.Fatal("preview executed wrapper")
			}
			var r collect.Report
			if err := json.Unmarshal(run(), &r); err != nil {
				t.Fatal(err)
			}
			if r.Status != "partial" || len(r.Devices) != 1 {
				t.Fatal(r)
			}
			rows, failed := 0, 0
			for _, c := range r.Devices[0].Commands {
				if c.Group == "optics" {
					if c.Status == "failed" {
						failed++
					} else if c.Facts != nil {
						rows += len(c.Facts.Optics)
					}
				}
			}
			if rows != 64 || failed != 1 {
				t.Fatal(rows, failed)
			}
		})
	}
}
