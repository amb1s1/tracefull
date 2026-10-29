package investigate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("TRACEFULL_PROCESS_TEST") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "success":
		fmt.Print("stdout")
		fmt.Fprint(os.Stderr, "stderr")
	case "failure":
		fmt.Fprint(os.Stderr, "permission denied")
		os.Exit(7)
	case "flood":
		fmt.Print(strings.Repeat("x", MaxOutputBytes+5000))
	case "descendant":
		child := exec.Command(os.Args[0], "-test.run=TestProcessHelper", "--", "hold")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(9)
		}
		time.Sleep(10 * time.Second)
	case "hold":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}
func TestBoundedProcess(t *testing.T) {
	t.Setenv("TRACEFULL_PROCESS_TEST", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	for _, tc := range []struct{ mode, status string }{{"success", "ok"}, {"failure", "failed"}, {"flood", "truncated"}, {"hold", "timeout"}, {"descendant", "timeout"}} {
		t.Run(tc.mode, func(t *testing.T) {
			limit := 2 * time.Second
			if tc.mode == "hold" || tc.mode == "descendant" {
				limit = 300 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			defer cancel()
			start := time.Now()
			r := (ProcessRunner{}).Run(ctx, Command{Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper", "--", tc.mode}})
			if r.Status != tc.status {
				t.Fatalf("%s: status=%s error=%s stdout=%d stderr=%d", tc.mode, r.Status, r.Error, len(r.Stdout), len(r.Stderr))
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("process cleanup exceeded deadline")
			}
			if len(r.Stdout) > MaxOutputBytes || len(r.Stderr) > MaxOutputBytes {
				t.Fatal("unbounded capture")
			}
			if tc.mode == "success" && (string(r.Stdout) != "stdout" || string(r.Stderr) != "stderr" || r.ExitCode == nil || *r.ExitCode != 0) {
				t.Fatal(r)
			}
		})
	}
	r := (ProcessRunner{}).Run(context.Background(), Command{Executable: "/missing/tracefull-tool"})
	if r.Status != "failed" || r.ExitCode != nil {
		t.Fatal(r)
	}
}
