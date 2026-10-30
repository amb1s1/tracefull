package device

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This policy belongs only to synthetic tests, not a supported device driver.
func testPolicy(command string) error {
	switch command {
	case "show version", "show wait", "show flood", "show fail":
		return nil
	}
	return fmt.Errorf("command is not in the synthetic policy")
}

func TestWrapperHelper(t *testing.T) {
	if os.Getenv("TRACEFULL_WRAPPER_TEST") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "show version":
		fmt.Print("synthetic version")
		fmt.Fprint(os.Stderr, "notice")
	case "show fail":
		os.Exit(7)
	case "show wait":
		fmt.Print("partial")
		time.Sleep(10 * time.Second)
	case "show flood":
		fmt.Print(strings.Repeat("x", (1<<20)+1))
	}
	os.Exit(0)
}

func TestWrapperPlan(t *testing.T) {
	target := Target{Host: "router.test", Address: "192.0.2.1", User: "reader", Jump: "jump.test", Platform: "synthetic"}
	template := fmt.Sprintf("%q -test.run=TestWrapperHelper -- {user}@{host} --address={address} --jump {jump} --platform {platform} {cmd}", os.Args[0])
	w, err := NewWrapper(template, target, testPolicy, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	argv, err := w.Plan("show version")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{os.Args[0], "-test.run=TestWrapperHelper", "--", "reader@router.test", "--address=192.0.2.1", "--jump", "jump.test", "--platform", "synthetic", "show version"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("%q != %q", argv, want)
	}
	argv[len(argv)-1] = "mutated"
	again, _ := w.Plan("show version")
	if again[len(again)-1] != "show version" {
		t.Fatal("plan aliases mutable state")
	}
	for _, command := range []string{"configure terminal", "show version; clear", "show version\nreload", "show version | sh", "show $(id)", ""} {
		if _, err := w.Plan(command); err == nil {
			t.Fatalf("accepted %q", command)
		}
	}
	for _, tail := range []string{"", "prefix{cmd}", "{cmd} {cmd}", "{unknown} {cmd}", "{cmd};id", "$(id) {cmd}", "`id` {cmd}", "'unterminated {cmd}", "{cmd}\nignored", "\\n {cmd}"} {
		if _, err := NewWrapper(fmt.Sprintf("%q %s", os.Args[0], tail), target, testPolicy, time.Second); err == nil {
			t.Fatalf("accepted template %q", tail)
		}
	}
	for _, host := range []string{"-oProxyCommand=id", "host;id", "host name", "$(id)", "{cmd}"} {
		target.Host = host
		if _, err := NewWrapper(template, target, testPolicy, time.Second); err == nil {
			t.Fatalf("accepted host %q", host)
		}
	}
	if _, err := NewWrapper(template, target, nil, time.Second); err == nil {
		t.Fatal("missing policy accepted")
	}
	if _, err := NewWrapper("/bin/sh -c {cmd}", Target{}, testPolicy, time.Second); err == nil {
		t.Fatal("explicit shell accepted")
	}
}

func TestWrapperLifecycle(t *testing.T) {
	t.Setenv("TRACEFULL_WRAPPER_TEST", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	template := fmt.Sprintf("%q -test.run=TestWrapperHelper -- {cmd}", os.Args[0])
	for _, tc := range []struct{ cmd, status string }{{"show version", "ok"}, {"show fail", "failed"}, {"show wait", "timeout"}, {"show flood", "truncated"}} {
		t.Run(tc.cmd, func(t *testing.T) {
			duration := 2 * time.Second
			if tc.cmd == "show wait" {
				duration = 200 * time.Millisecond
			}
			w, err := NewWrapper(template, Target{}, testPolicy, duration)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = w.Close() }()
			r, err := w.Run(context.Background(), tc.cmd)
			if err != nil || r.Status != tc.status {
				t.Fatalf("status %s error %v / %s", r.Status, err, r.Error)
			}
			if len(r.Stdout) > 1<<20 || r.StartedAt.IsZero() || r.EndedAt.Before(r.StartedAt) {
				t.Fatal("invalid result bounds")
			}
			if tc.cmd == "show version" && (string(r.Stdout) != "synthetic version" || string(r.Stderr) != "notice") {
				t.Fatal("missing output")
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Run(context.Background(), "show version"); err == nil {
				t.Fatal("ran closed session")
			}
		})
	}
}

func TestWrapperNoExpansion(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "unexpected")
	template := fmt.Sprintf("%q $(touch %s) {cmd}", os.Args[0], marker)
	_, err := NewWrapper(template, Target{}, testPolicy, time.Second)
	if err == nil {
		t.Fatal("substitution accepted")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("substitution executed")
	}
}

func TestFixedDisplaySuffixPolicy(t *testing.T) {
	// A deliberately permissive test policy isolates the common syntax boundary.
	allow := func(string) error { return nil }
	for _, cmd := range []string{"show version | json", "show version | display json"} {
		if err := validate(allow, cmd); err != nil {
			t.Fatal(err)
		}
		if err := validate(testPolicy, cmd); err == nil {
			t.Fatal("suffix bypassed full platform policy")
		}
	}
	for _, cmd := range []string{"show version | sh", "show version | display json | json", "show version | json | display json", "show version; id | json", "show $(id) | json", "show version\n | json", "show version | json > /tmp/out"} {
		if err := validate(allow, cmd); err == nil {
			t.Fatalf("accepted %q", cmd)
		}
	}
}
