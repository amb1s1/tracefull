package device

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/amb1s1/tracefull/internal/process"
)

type SSHOptions struct {
	Address, User, KnownHosts, AgentSocket string
	IdentityFiles                          []string
	DefaultIdentityDir                     string
	InsecureHostKey                        bool
	ConnectTimeout, CommandTimeout         time.Duration
	Jump                                   *SSHOptions
}

type SSHSession struct {
	client  *ssh.Client
	conn    net.Conn
	jump    *SSHSession
	policy  Policy
	timeout time.Duration
	gate    chan struct{}
	closed  chan struct{}
	once    sync.Once
}

// DialSSH verifies host keys before opening command channels. Jump connections
// receive their own authentication and host-key configuration, never fallback.
func DialSSH(ctx context.Context, o SSHOptions, policy Policy) (*SSHSession, error) {
	if policy == nil {
		return nil, fmt.Errorf("device has no command policy")
	}
	if o.ConnectTimeout == 0 {
		o.ConnectTimeout = 5 * time.Second
	}
	if o.CommandTimeout == 0 {
		o.CommandTimeout = 25 * time.Second
	}
	if err := timeoutValid(o.ConnectTimeout); err != nil {
		return nil, err
	}
	if err := timeoutValid(o.CommandTimeout); err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(o.Address); err != nil {
		return nil, fmt.Errorf("SSH address requires host:port: %w", err)
	}
	if o.User == "" {
		return nil, fmt.Errorf("SSH user required")
	}
	if o.Jump != nil && o.Jump.Jump != nil {
		return nil, fmt.Errorf("only one SSH jump host is supported")
	}
	ctx, cancel := context.WithTimeout(ctx, o.ConnectTimeout)
	defer cancel()
	callback, err := hostKeys(o)
	if err != nil {
		return nil, err
	}
	auth, cleanup, err := authentication(ctx, o)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	config := &ssh.ClientConfig{User: o.User, Auth: auth, HostKeyCallback: callback}
	var conn net.Conn
	var jump *SSHSession
	if o.Jump == nil {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", o.Address)
	} else {
		jump, err = DialSSH(ctx, *o.Jump, policy)
		if err == nil {
			conn, err = jump.client.DialContext(ctx, "tcp", o.Address)
		}
	}
	if err != nil {
		if jump != nil {
			_ = jump.Close()
		}
		return nil, fmt.Errorf("SSH connect: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = closeConnection(conn, jump) })
	defer stop()
	c, channels, requests, err := ssh.NewClientConn(conn, o.Address, config)
	if err != nil {
		_ = closeConnection(conn, jump)
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}
	s := &SSHSession{client: ssh.NewClient(c, channels, requests), conn: conn, jump: jump, policy: policy, timeout: o.CommandTimeout, gate: make(chan struct{}, 1), closed: make(chan struct{})}
	if ctx.Err() != nil {
		_ = s.Close()
		return nil, ctx.Err()
	}
	return s, nil
}

func hostKeys(o SSHOptions) (ssh.HostKeyCallback, error) {
	if o.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	} // Explicit opt-in only.
	if o.KnownHosts == "" {
		return nil, fmt.Errorf("known-hosts file required unless insecure host keys are explicitly enabled")
	}
	if err := regularSSHFile(o.KnownHosts); err != nil {
		return nil, fmt.Errorf("known-hosts file: %w", err)
	}
	return knownhosts.New(o.KnownHosts)
}

func authentication(ctx context.Context, o SSHOptions) ([]ssh.AuthMethod, func(), error) {
	signers := []ssh.Signer{}
	cleanup := func() {}
	var agentErr error
	if o.AgentSocket != "" {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", o.AgentSocket)
		agentErr = err
		if err == nil {
			deadline, _ := ctx.Deadline()
			_ = conn.SetDeadline(deadline)
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			cleanup = func() { stop(); _ = conn.Close() }
			signers, agentErr = agent.NewClient(conn).Signers()
			if agentErr != nil {
				cleanup()
				signers = nil
			}
		}
	}
	paths := o.IdentityFiles
	defaults := len(paths) == 0 && o.DefaultIdentityDir != ""
	if defaults {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			paths = append(paths, filepath.Join(o.DefaultIdentityDir, name))
		}
	}
	for _, path := range paths {
		key, err := readKey(path)
		if err != nil {
			var encrypted *ssh.PassphraseMissingError
			if defaults && (errors.Is(err, os.ErrNotExist) || errors.As(err, &encrypted)) {
				continue
			}
			cleanup()
			return nil, cleanup, err
		}
		signers = append(signers, key)
	}
	if len(signers) == 0 {
		cleanup()
		return nil, cleanup, fmt.Errorf("no SSH authentication available (agent: %v)", agentErr)
	}
	// The SSH client attempts each authentication method only once. Keep all
	// public keys in one method, preserving agent-first identity ordering.
	return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, cleanup, nil
}

func readKey(path string) (ssh.Signer, error) {
	if err := regularSSHFile(path); err != nil {
		return nil, fmt.Errorf("SSH identity: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open SSH identity: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 1<<20+1))
	err = errors.Join(readErr, f.Close())
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("SSH identity exceeds 1 MiB")
	}
	key, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("SSH identity must be usable without a password prompt: %w", err)
	}
	return key, nil
}

// Local SSH configuration is file-based. Pipes/devices can wait indefinitely
// before the context-aware network connection has even started. Stat follows
// symlinks so conventional links to regular key and known-hosts files still work.
func regularSSHFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q must be a regular file", path)
	}
	return nil
}

type sshOutput struct {
	buf       bytes.Buffer
	truncated bool
	stop      func()
}

func (b *sshOutput) Write(p []byte) (int, error) {
	left := process.MaxOutputBytes - b.buf.Len()
	if len(p) > left {
		_, _ = b.buf.Write(p[:left])
		b.truncated = true
		b.stop()
		return left, fmt.Errorf("SSH output limit exceeded")
	}
	return b.buf.Write(p)
}

func (s *SSHSession) Run(ctx context.Context, command string) (Result, error) {
	if err := validate(s.policy, command); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := acquire(ctx, s.gate); err != nil {
		return Result{}, err
	}
	defer func() { <-s.gate }()
	select {
	case <-s.closed:
		return Result{}, fmt.Errorf("SSH session closed")
	default:
	}
	r := Result{Command: command, StartedAt: time.Now().UTC()}
	// Closing the underlying connection also unblocks channel open and output
	// copying. A timed-out session is not reused or silently reconnected.
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()
	channel, err := s.client.NewSession()
	out := &sshOutput{stop: func() { _ = s.Close() }}
	errout := &sshOutput{stop: func() { _ = s.Close() }}
	if err == nil {
		channel.Stdout, channel.Stderr = out, errout
		err = channel.Run(command)
		_ = channel.Close()
	}
	r.EndedAt = time.Now().UTC()
	r.Stdout, r.Stderr = out.buf.Bytes(), errout.buf.Bytes()
	r.Status = "ok"
	if err == nil {
		code := 0
		r.ExitCode = &code
	}
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitStatus()
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
	return r, nil
}

func (s *SSHSession) Close() error {
	var err error
	s.once.Do(func() {
		close(s.closed)
		// Close the socket first; SSH close writes must not block cleanup.
		err = closeConnection(s.conn, s.jump)
		_ = s.client.Close()
	})
	return err
}

func closeConnection(conn net.Conn, jump *SSHSession) error {
	// A forwarded net.Conn closes through the jump's SSH writer. Close that
	// transport first so a stalled tunnel cannot block cancellation cleanup.
	var err error
	if jump != nil {
		err = jump.Close()
	}
	closeErr := conn.Close()
	// Closing the jump intentionally tears down its forwarded channels. Their
	// subsequent close may observe EOF or the already-closed transport; neither
	// means a successful collection lost evidence during cleanup.
	if errors.Is(closeErr, net.ErrClosed) || (jump != nil && errors.Is(closeErr, io.EOF)) {
		closeErr = nil
	}
	return errors.Join(err, closeErr)
}
