package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/collect"
)

func testEOSRouteExecutable(t *testing.T, bin string) {
	fixtures, err := filepath.Abs("../../internal/driver/eos/testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, malformed := range []bool{false, true} {
		dir := t.TempDir()
		// Synthetic trace, inventory and malformed variant; the valid response is a real capture.
		raw, err := os.ReadFile(filepath.Join(fixtures, "route4-empty.json"))
		if err != nil {
			t.Fatal(err)
		}
		if malformed {
			raw = []byte(`{"vrfs":{"default":{"routes":{"0.0.0.0/0":null}}}}`)
		}
		script := "#!/bin/sh\ncase \"$2\" in\n 'show ip route 1.1.1.1 | json') exec /bin/cat response.json ;;\n *) exit 9 ;;\nesac\n"
		for name, data := range map[string][]byte{
			"replay": []byte(script), "response.json": raw,
			"inventory.yaml": []byte("devices:\n  - name: switch-a\n    address: 192.0.2.1\n    platform: eos\n"),
			"trace.txt":      []byte("traceroute to 1.1.1.1 (1.1.1.1), 30 hops max\n1 192.0.2.1 1 ms\n"),
		} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0700); err != nil {
				t.Fatal(err)
			}
		}
		for _, format := range []string{"json", "text", "markdown"} {
			evidenceDir := filepath.Join(dir, "evidence-"+format)
			c := exec.Command(bin, "collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--collect", "route", "--ssh-cmd", filepath.Join(dir, "replay")+" {address} {cmd}", "--format", format, "--no-live", "--evidence-dir", evidenceDir)
			c.Dir = dir
			c.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "NO_COLOR=1"}
			out, err := c.Output()
			var exit *exec.ExitError
			if malformed {
				if !errors.As(err, &exit) || exit.ExitCode() != 3 {
					t.Fatalf("malformed: %v %s", err, out)
				}
			} else if err != nil {
				t.Fatalf("empty lookup: %v %s", err, out)
			}
			if format == "json" {
				var r collect.Report
				if err := json.Unmarshal(out, &r); err != nil {
					t.Fatal(err)
				}
				if len(r.Devices) != 1 || len(r.Devices[0].Commands) != 1 || len(r.Devices[0].Interfaces) != 0 {
					t.Fatal(r)
				}
				command := r.Devices[0].Commands[0]
				if malformed {
					if command.Status != "parse_failed" || command.Facts != nil {
						t.Fatal(command)
					}
				} else if r.Status != "complete" || command.Status != "ok" || command.Facts == nil || len(command.Facts.Routes) != 0 {
					t.Fatal(r)
				}
				if command.Stdout == nil {
					t.Fatal("missing raw evidence")
				}
				saved, err := os.ReadFile(filepath.Join(evidenceDir, command.Stdout.Path))
				if err != nil || !bytes.Equal(raw, saved) {
					t.Fatal("raw response not retained", err)
				}
			} else if malformed && !strings.Contains(html.UnescapeString(string(out)), "parse_failed") {
				t.Fatalf("failure hidden: %s", out)
			}
		}
	}
}
