package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tracefull")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	t.Run("optics", func(t *testing.T) { testOpticsExecutable(t, bin) })
	t.Run("management", func(t *testing.T) { testManagementExecutable(t, bin) })
	t.Run("platforms", func(t *testing.T) { testPlatformExecutables(t, bin) })
	fixture, err := filepath.Abs("../../internal/driver/cumulus/testdata")
	if err != nil {
		t.Fatal(err)
	}
	// This trusted synthetic wrapper replays real scrubbed captures; it never opens SSH.
	wrapper := filepath.Join(dir, "fake-device")
	script := `#!/bin/sh
printf ran > "$TRACEFULL_TEST_MARKER"
case "$2" in
 "cat /etc/os-release") exec /bin/cat "$TRACEFULL_TEST_FIXTURES/os-release.txt" ;;
 "cat /proc/uptime") exec /bin/cat "$TRACEFULL_TEST_FIXTURES/uptime.txt" ;;
 *) printf 'denied\n' >&2; exit 9 ;;
esac
`
	for name, data := range map[string]string{"fake-device": script, "inventory.yaml": "devices:\n  - name: switch-a\n    address: 192.0.2.1\n    platform: cumulus\n", "trace.txt": "traceroute to dst (192.0.2.2), 30 hops max\n1 192.0.2.1 1 ms\n2 192.0.2.2 2 ms\n"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(dir, "marker")
	env := []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "NO_COLOR=1", "TRACEFULL_TEST_MARKER=" + marker, "TRACEFULL_TEST_FIXTURES=" + fixture}
	base := []string{"collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--ssh-cmd", wrapper + " {address} {cmd}", "--collect", "version", "--json"}
	run := func(extra ...string) ([]byte, error) {
		c := exec.Command(bin, append(append([]string{}, base...), extra...)...)
		c.Dir = dir
		c.Env = env
		out, err := c.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				t.Logf("CLI stderr: %s", exit.Stderr)
			}
		}
		return out, err
	}
	out, err := run("--dry-run")
	if err != nil || !json.Valid(out) {
		t.Fatalf("dry run: %v %s", err, out)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("dry run executed wrapper")
	}
	out, err = run("--evidence-dir", filepath.Join(dir, "evidence"))
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	var doc struct {
		Status  string
		Devices []struct {
			Commands []struct {
				Argv   []string
				Status string
			}
		}
	}
	if err = json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != "complete" || len(doc.Devices) != 1 || len(doc.Devices[0].Commands) != 2 {
		t.Fatal(doc)
	}
	for _, c := range doc.Devices[0].Commands {
		if len(c.Argv) != 3 || !strings.HasPrefix(c.Argv[2], "cat ") || c.Status != "ok" {
			t.Fatal(c)
		}
	}
	base[len(base)-2] = "route"
	out, err = run("--evidence-dir", filepath.Join(dir, "failed"))
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || !json.Valid(out) {
		t.Fatalf("no results: %v %s", err, out)
	}
	for _, extra := range [][]string{{"--extra-cmd", "sudo reboot"}, {"--vrf", "tenant-a"}, {"--collect", "typo"}, {"--markdown"}, {"--workers", "0"}} {
		_, err = run(append(extra, "--dry-run")...)
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			t.Fatalf("usage %v: %v", extra, err)
		}
	}
	c := exec.Command(bin, "schema", "--kind", "collection")
	out, err = c.Output()
	if err != nil || !json.Valid(out) {
		t.Fatalf("schema: %v %s", err, out)
	}
}

func testPlatformExecutables(t *testing.T, bin string) {
	for _, tc := range []struct{ platform, dialect, command, file, version string }{
		{"frr", "cumulus", "vtysh -c 'show version'", "frr-version.txt", "10.0.3"},
		{"linux", "cumulus", "vtysh -c 'show version'", "frr-version.txt", "10.0.3"},
		{"arista-eos", "eos", "show version | json", "version-json.json", "4.33.4M"},
		{"junos", "junos", "show version | display json", "version-json.json", "24.4R2-S5.3"},
		{"cisco-ios-xr", "iosxr", "show version", "version.txt", "25.4.2"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			dir := t.TempDir()
			fixture, err := filepath.Abs("../../internal/driver/" + tc.dialect + "/testdata")
			if err != nil {
				t.Fatal(err)
			}
			wrapper := filepath.Join(dir, "replay")
			script := "#!/bin/sh\ncase \"$2\" in\n " + "\"" + tc.command + "\") exec /bin/cat \"$CAPTURES/" + tc.file + "\" ;;\n \"show system uptime | display json\") exec /bin/cat \"$CAPTURES/uptime.json\" ;;\n \"cat /etc/os-release\") exec /bin/cat \"$CAPTURES/os-release.txt\" ;;\n \"cat /proc/uptime\") exec /bin/cat \"$CAPTURES/uptime.txt\" ;;\n *) exit 9 ;;\nesac\n"
			for name, data := range map[string]string{"replay": script, "inventory.yaml": "devices:\n  - name: device-a\n    address: 192.0.2.1\n    platform: " + tc.platform + "\n", "trace.txt": "traceroute to 192.0.2.2 (192.0.2.2), 30 hops max\n1 192.0.2.1 1 ms\n"} {
				if err = os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, format := range []string{"--json", "--markdown", "--no-live"} {
				c := exec.Command(bin, "collect", "--from", "trace.txt", "--inventory", "inventory.yaml", "--collect", "version", "--ssh-cmd", wrapper+" {address} {cmd}", format)
				c.Dir = dir
				c.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "PATH=/usr/bin:/bin", "NO_COLOR=1", "CAPTURES=" + fixture}
				out, err := c.CombinedOutput()
				if err != nil || !strings.Contains(string(out), tc.version) {
					t.Fatalf("%v: %s", err, out)
				}
				if format == "--json" && !json.Valid(out) {
					t.Fatal("invalid JSON")
				}
				if strings.Contains(string(out), "\x1b") {
					t.Fatal("ANSI leaked")
				}
			}
		})
	}
}
