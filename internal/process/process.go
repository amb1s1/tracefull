//go:build linux || darwin

// Package process runs bounded local process groups without a shell.
package process

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const MaxOutputBytes = 1 << 20

type Result struct {
	Stdout, Stderr []byte
	ExitCode       *int
	Status, Error  string
}

type cappedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
	cancel    context.CancelFunc
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(p) > remaining {
		_, _ = b.buffer.Write(p[:remaining])
		b.truncated = true
		b.cancel()
		return remaining, errors.New("output limit exceeded")
	}
	return b.buffer.Write(p)
}

// Run owns a process group. Cancellation terminates the group and WaitDelay
// bounds inherited output pipes even when a descendant retains a descriptor.
func Run(ctx context.Context, executable string, args, env []string) Result {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	out := cappedOutput{limit: MaxOutputBytes, cancel: cancel}
	errout := cappedOutput{limit: MaxOutputBytes, cancel: cancel}
	cmd := exec.CommandContext(child, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	cmd.Stdout, cmd.Stderr = &out, &errout
	cmd.Env = env
	err := cmd.Run()
	// Reap remaining members if the direct child exited while descendants kept pipes open.
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	r := Result{Stdout: out.buffer.Bytes(), Stderr: errout.buffer.Bytes(), Status: "ok"}
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		r.ExitCode = &code
	}
	switch {
	case out.truncated || errout.truncated:
		r.Status, r.Error = "truncated", "output exceeded 1 MiB per stream"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		r.Status, r.Error = "timeout", ctx.Err().Error()
	case ctx.Err() != nil:
		r.Status, r.Error = "cancelled", ctx.Err().Error()
	case err != nil:
		r.Status, r.Error = "failed", err.Error()
	}
	return r
}
