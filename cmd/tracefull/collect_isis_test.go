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

func testISISExecutable(t *testing.T, bin string) {
	dir := t.TempDir()
	fixtures, err := filepath.Abs("../../internal/driver/iosxr/testdata")
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic trace/transport; the command output is an unchanged real capture.
	script := `#!/bin/sh
case "$2" in
 "show route ipv6 2001:db8:30::32 detail") exec /bin/cat "$CAPTURES/route6-isis-multipath.txt" ;;
 *) exit 9 ;;
esac
`
	for name, data := range map[string]string{
		"replay":         script,
		"inventory.yaml": "devices:\n  - name: router-a\n    address: 2001:db8::1\n    platform: cisco-ios-xr\n",
		"trace.txt":      "traceroute to 2001:db8:30::32 (2001:db8:30::32), 30 hops max\n1 2001:db8::1 1 ms\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, format := range []string{"json", "text", "markdown"} {
		c := exec.Command(bin, "collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--collect", "route", "--ssh-cmd", filepath.Join(dir, "replay")+" {address} {cmd}", "--format", format, "--no-live")
		c.Dir = dir
		c.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "CAPTURES=" + fixtures}
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", format, err, out)
		}
		if format == "json" {
			var r collect.Report
			if err := json.Unmarshal(out, &r); err != nil {
				t.Fatal(err)
			}
			if r.Status != "complete" || len(r.Devices) != 1 || len(r.Devices[0].Interfaces) != 2 {
				t.Fatal(r)
			}
			route := r.Devices[0].Commands[0].Facts.Routes[0]
			if route.Protocol != "isis 1" || route.Installed != nil || len(route.NextHops) != 2 || route.NextHops[0].Address != "fe80::1" {
				t.Fatal(route)
			}
		} else {
			for _, want := range []string{"protocol: isis 1", "Next hop fe80::1 on FourHundredGigE0/0/0/4", "Next hop fe80::2 on FourHundredGigE0/0/0/5", "installed: ?"} {
				if !strings.Contains(string(out), want) {
					t.Fatalf("missing %q: %s", want, out)
				}
			}
		}
	}
}
