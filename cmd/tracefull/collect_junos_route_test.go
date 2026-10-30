package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/collect"
)

func testJunosRouteExecutable(t *testing.T, bin string) {
	dir := t.TempDir()
	fixtures, err := filepath.Abs("../../internal/driver/junos/testdata")
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic trace and transport replay the real scrubbed device response offline.
	script := `#!/bin/sh
case "$2" in
 "show route table inet.0 192.0.2.10 detail | display json") exec /bin/cat "$CAPTURES/route4-isis-detail.json" ;;
 *) exit 9 ;;
esac
`
	for name, data := range map[string]string{
		"replay":         script,
		"inventory.yaml": "devices:\n  - name: router-a\n    address: 192.0.2.1\n    platform: junos\n",
		"trace.txt":      "traceroute to 192.0.2.10 (192.0.2.10), 30 hops max\n1 192.0.2.1 1 ms\n",
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
			if r.Status != "complete" || len(r.Devices) != 1 || !slices.Equal(r.Devices[0].Interfaces, []string{"et-0/0/0", "et-0/0/0.0"}) {
				t.Fatal(r)
			}
			route := r.Devices[0].Commands[0].Facts.Routes[0]
			if route.Protocol != "IS-IS" || route.Installed != nil || len(route.NextHops) != 2 || route.NextHops[1].Active != nil {
				t.Fatal(route)
			}
		} else {
			for _, want := range []string{"protocol: IS-IS", "Next hop 198.51.100.97 on et-0/0/0.0 | active: true | installed: ?", "Next hop 198.51.100.99 on et-0/1/0.0 | active: ? | installed: ?"} {
				if !strings.Contains(string(out), want) {
					t.Fatalf("missing %q: %s", want, out)
				}
			}
		}
	}
}
