package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInvestigateExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tracefull")
	toolsDir := filepath.Join(dir, "tools")
	if err := os.Mkdir(toolsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	// Synthetic executables emit fixed observations and never send packets.
	script := "#!/bin/sh\nprintf ran > \"$TRACEFULL_TEST_MARKER\"\nprintf 'traceroute to dst (192.0.2.2), 30 hops max\\n' >&2\nprintf '1 192.0.2.2 2 ms\\n'\n"
	if err := os.WriteFile(filepath.Join(toolsDir, "traceroute"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "mtr"), []byte("#!/bin/sh\nprintf 'permission denied\\n' >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "started")
	env := []string{"PATH=" + toolsDir, "HOME=" + dir, "XDG_CONFIG_HOME=" + filepath.Join(dir, "config"), "TRACEFULL_TEST_MARKER=" + marker, "NO_COLOR=1"}
	run := func(args ...string) ([]byte, error) {
		c := exec.Command(bin, args...)
		c.Dir = dir
		c.Env = env
		return c.Output()
	}
	out, err := run("investigate", "192.0.2.2", "--dry-run", "--json")
	if err != nil || !json.Valid(out) {
		t.Fatalf("dry-run: %s %v", out, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("dry-run executed a tool")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".tracefull-evidence-*"))
	if len(matches) > 0 {
		t.Fatal("dry-run wrote evidence")
	}
	evidence := filepath.Join(dir, "evidence")
	out, err = run("investigate", "192.0.2.2", "--tools", "traceroute,mtr", "--json", "--evidence-dir", evidence)
	if err != nil {
		t.Fatalf("partial run: %s %v", out, err)
	}
	var doc struct {
		Status  string
		Results []struct{ Status string }
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != "partial" || len(doc.Results) != 2 || doc.Results[0].Status != "ok" || doc.Results[1].Status != "failed" {
		t.Fatal(doc)
	}
	out, err = run("investigate", "192.0.2.2", "--tools", "mtr", "--json", "--evidence-dir", filepath.Join(dir, "failed"))
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || !json.Valid(out) {
		t.Fatalf("all failed: %v %s", err, out)
	}
	_, err = run("investigate", "192.0.2.2", "--evidence-dir", evidence)
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("overwrite: %v", err)
	}
	// Wait for the fake process to start before signalling its owning CLI.
	blocking := "#!/bin/sh\nprintf started > \"$TRACEFULL_TEST_MARKER\"\nprintf 'partial output\\n'\n/bin/sleep 20\n"
	if err := os.WriteFile(filepath.Join(toolsDir, "traceroute"), []byte(blocking), 0700); err != nil {
		t.Fatal(err)
	}
	marker2 := filepath.Join(dir, "blocking-started")
	blockedDir := filepath.Join(dir, "interrupted")
	c := exec.Command(bin, "investigate", "192.0.2.2", "--tools", "traceroute", "--json", "--evidence-dir", blockedDir)
	c.Dir = dir
	c.Env = append(env, "TRACEFULL_TEST_MARKER="+marker2)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if c.ProcessState == nil {
			_ = c.Process.Kill()
			_ = c.Wait()
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker2); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake tool did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		if !errors.As(err, &exit) || exit.ExitCode() != 130 {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("investigation did not cancel")
	}
	data, err := os.ReadFile(filepath.Join(blockedDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !strings.Contains(string(data), `"status": "interrupted"`) {
		t.Fatal("missing partial report")
	}
}
