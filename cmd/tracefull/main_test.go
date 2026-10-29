package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestExecutable exercises actual process output, exit codes and blocked-stdin cleanup.
func TestExecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tracefull")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", out, e)
	}
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "TRACEFULL_") && !strings.HasPrefix(v, "XDG_CONFIG_HOME=") {
			env = append(env, v)
		}
	}
	env = append(env, "XDG_CONFIG_HOME="+filepath.Join(dir, "config"), "NO_COLOR=1")
	input := "traceroute to dst (192.0.2.1), 30 hops max\n1 192.0.2.1 1 ms !X\n"
	for _, tc := range []struct {
		args   []string
		code   int
		report bool
	}{{[]string{"enrich", "--from", "-", "--json"}, 0, true}, {[]string{"enrich", "--from", "-", "--json", "--fail-on", "icmp-error"}, 1, true}, {[]string{"enrich", "--from", "missing"}, 2, false}, {[]string{"unknown"}, 2, false}, {[]string{"enrich", "--from", "-", "--rdns-timeout", "bad"}, 2, false}} {
		c := exec.Command(bin, tc.args...)
		c.Dir = dir
		c.Env = env
		c.Stdin = strings.NewReader(input)
		var out, errout bytes.Buffer
		c.Stdout = &out
		c.Stderr = &errout
		e := c.Run()
		code := 0
		var ee *exec.ExitError
		if errors.As(e, &ee) {
			code = ee.ExitCode()
		} else if e != nil {
			t.Fatal(e)
		}
		if code != tc.code {
			t.Fatalf("%v: exit %d stderr=%s", tc.args, code, &errout)
		}
		if tc.report {
			if !json.Valid(out.Bytes()) || errout.Len() != 0 {
				t.Fatalf("report separation: %s %s", &out, &errout)
			}
		} else if out.Len() != 0 || errout.Len() == 0 {
			t.Fatal("error output contract")
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	c := exec.Command(bin, "enrich", "--from", "-")
	c.Env = env
	c.Dir = dir
	in, e := c.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = in.Close() }()
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(150 * time.Millisecond)
	if e = c.Process.Signal(os.Interrupt); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case e := <-done:
		var ee *exec.ExitError
		if !errors.As(e, &ee) || ee.ExitCode() != 130 {
			t.Fatalf("interrupt exit: %v", e)
		}
	case <-time.After(3 * time.Second):
		_ = c.Process.Kill()
		if e := <-done; e == nil {
			t.Error("killed process unexpectedly succeeded")
		}
		t.Fatal("blocked input did not cancel")
	}
}

func TestBlockedOutputInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows signal behavior requires its native console runner")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tracefull")
	if out, e := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); e != nil {
		t.Fatalf("%s %v", out, e)
	}
	var input strings.Builder
	input.WriteString("traceroute to dst (192.0.2.255), 255 hops max\n")
	for i := 1; i <= 255; i++ {
		fmt.Fprintf(&input, "%d 192.0.2.%d 1 ms\n", i, i)
	}
	c := exec.Command(bin, "enrich", "--from", "-", "--json")
	c.Dir = dir
	c.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(dir, "config"))
	c.Stdin = strings.NewReader(input.String())
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	c.Stdout = writer
	if e = c.Start(); e != nil {
		t.Fatal(e)
	}
	if e = reader.SetReadDeadline(time.Now().Add(3 * time.Second)); e != nil {
		t.Fatal(e)
	}
	first := make([]byte, 1)
	if _, e = reader.Read(first); e != nil {
		_ = c.Process.Kill()
		_ = c.Wait()
		t.Fatalf("no initial output: %v", e)
	}
	if e = c.Process.Signal(os.Interrupt); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case e := <-done:
		var ee *exec.ExitError
		if !errors.As(e, &ee) || ee.ExitCode() != 130 {
			t.Fatalf("interrupt while writing: %v", e)
		}
	case <-time.After(3 * time.Second):
		_ = c.Process.Kill()
		if e := <-done; e == nil {
			t.Error("killed process succeeded")
		}
		t.Fatal("full stdout pipe did not cancel")
	}
}
