package device

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testKey(t *testing.T) (ssh.Signer, string) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	return signer, path
}

// The test server accepts only a generated key and synthetic commands. It binds
// loopback; no device output or production credentials are used.
func fakeSSH(t *testing.T) (SSHOptions, *atomic.Int32) {
	t.Helper()
	host, _ := testKey(t)
	client, keyfile := testKey(t)
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if string(key.Marshal()) != string(client.PublicKey().Marshal()) {
			return nil, fmt.Errorf("wrong key")
		}
		return nil, nil
	}}
	config.AddHostKey(host)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(known, []byte(knownhosts.Line([]string{l.Addr().String()}, host.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	connections := []net.Conn{}
	count := &atomic.Int32{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections = append(connections, conn)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { _ = conn.Close() }()
				s, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				wg.Add(1)
				go func() { defer wg.Done(); _ = s.Wait(); cancel() }()
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					if newChannel.ChannelType() == "direct-tcpip" {
						wg.Add(1)
						go func() { defer wg.Done(); forwardLocal(ctx, newChannel) }()
						continue
					}
					if newChannel.ChannelType() != "session" {
						_ = newChannel.Reject(ssh.UnknownChannelType, "test supports session only")
						continue
					}
					channel, reqs, err := newChannel.Accept()
					if err != nil {
						continue
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer func() { _ = channel.Close() }()
						for req := range reqs {
							if req.Type != "exec" {
								_ = req.Reply(false, nil)
								continue
							}
							var payload struct{ Command string }
							if ssh.Unmarshal(req.Payload, &payload) != nil {
								_ = req.Reply(false, nil)
								return
							}
							count.Add(1)
							_ = req.Reply(true, nil)
							code := uint32(0)
							switch payload.Command {
							case "show version":
								_, _ = io.WriteString(channel, "synthetic version")
								_, _ = io.WriteString(channel.Stderr(), "notice")
							case "show wait":
								_, _ = io.WriteString(channel, "partial")
								<-ctx.Done()
								return
							case "show flood":
								_, _ = io.WriteString(channel, strings.Repeat("x", (1<<20)+100))
							case "show fail":
								code = 7
							default:
								code = 9
							}
							status := make([]byte, 4)
							binary.BigEndian.PutUint32(status, code)
							_, _ = channel.SendRequest("exit-status", false, status)
							return
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		for _, c := range connections {
			_ = c.Close()
		}
		mu.Unlock()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("fake SSH server leaked")
		}
	})
	return SSHOptions{Address: l.Addr().String(), User: "reader", KnownHosts: known, IdentityFiles: []string{keyfile}, ConnectTimeout: time.Second, CommandTimeout: time.Second}, count
}

func forwardLocal(ctx context.Context, request ssh.NewChannel) {
	var destination struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}
	if ssh.Unmarshal(request.ExtraData(), &destination) != nil || destination.Host != "127.0.0.1" {
		_ = request.Reject(ssh.Prohibited, "only loopback forwarding in tests")
		return
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(destination.Host, strconv.Itoa(int(destination.Port))))
	if err != nil {
		_ = request.Reject(ssh.ConnectionFailed, "test connect failed")
		return
	}
	defer func() { _ = conn.Close() }()
	channel, requests, err := request.Accept()
	if err != nil {
		return
	}
	defer func() { _ = channel.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); _ = channel.Close() })
	defer stop()
	go ssh.DiscardRequests(requests)
	done := make(chan struct{})
	go func() { _, _ = io.Copy(conn, channel); _ = conn.Close(); close(done) }()
	_, _ = io.Copy(channel, conn)
	_ = channel.Close()
	<-done
}

func TestSSHSession(t *testing.T) {
	o, count := fakeSSH(t)
	s, err := DialSSH(context.Background(), o, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for range 2 {
		r, err := s.Run(context.Background(), "show version")
		if err != nil || r.Status != "ok" || string(r.Stdout) != "synthetic version" || string(r.Stderr) != "notice" || r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatalf("bad result: %+v %v", r, err)
		}
	}
	if _, err := s.Run(context.Background(), "configure terminal"); err == nil {
		t.Fatal("mutating command accepted")
	}
	if count.Load() != 2 {
		t.Fatal("policy rejection contacted server")
	}
	r, err := s.Run(context.Background(), "show fail")
	if err != nil || r.Status != "failed" || r.ExitCode == nil || *r.ExitCode != 7 {
		t.Fatal(r, err)
	}
}

func TestSSHBounds(t *testing.T) {
	for _, tc := range []struct{ command, status string }{{"show wait", "timeout"}, {"show flood", "truncated"}} {
		t.Run(tc.command, func(t *testing.T) {
			o, _ := fakeSSH(t)
			o.CommandTimeout = 300 * time.Millisecond
			s, err := DialSSH(context.Background(), o, testPolicy)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			start := time.Now()
			r, err := s.Run(context.Background(), tc.command)
			if err != nil || r.Status != tc.status {
				t.Fatalf("status=%s detail=%s err=%v", r.Status, r.Error, err)
			}
			if len(r.Stdout) > 1<<20 || time.Since(start) > 2*time.Second {
				t.Fatal("unbounded output or cancellation")
			}
			if _, err := s.Run(context.Background(), "show version"); err == nil {
				t.Fatal("dead session reused")
			}
		})
	}
}

func TestSSHHostKeysAndAuth(t *testing.T) {
	o, count := fakeSSH(t)
	wrong, _ := testKey(t)
	if err := os.WriteFile(o.KnownHosts, []byte(knownhosts.Line([]string{o.Address}, wrong.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DialSSH(context.Background(), o, testPolicy); err == nil {
		t.Fatal("wrong host key accepted")
	}
	if count.Load() != 0 {
		t.Fatal("host key failure executed command")
	}
	o.KnownHosts = ""
	if _, err := DialSSH(context.Background(), o, testPolicy); err == nil {
		t.Fatal("missing host keys accepted")
	}
	o.InsecureHostKey = true
	s, err := DialSSH(context.Background(), o, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	o.IdentityFiles = nil
	if _, err := DialSSH(context.Background(), o, testPolicy); err == nil {
		t.Fatal("unauthenticated session accepted")
	}
}

func TestAgentThenIdentity(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp")
	o, _ := fakeSSH(t)
	_, wrongPath := testKey(t)
	data, err := os.ReadFile(wrongPath)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	o.AgentSocket = filepath.Join(t.TempDir(), "agent")
	l, err := net.Listen("unix", o.AgentSocket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_ = agent.ServeAgent(keyring, c)
	}()
	s, err := DialSSH(context.Background(), o, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent connection was not closed")
	}
}

func TestSSHJump(t *testing.T) {
	target, _ := fakeSSH(t)
	jump, _ := fakeSSH(t)
	target.Jump = &jump
	s, err := DialSSH(context.Background(), target, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(context.Background(), "show version")
	if err != nil || r.Status != "ok" {
		t.Fatal(r, err)
	}
	_ = s.Close()
	wrong, _ := testKey(t)
	if err := os.WriteFile(jump.KnownHosts, []byte(knownhosts.Line([]string{jump.Address}, wrong.PublicKey())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DialSSH(context.Background(), target, testPolicy); err == nil {
		t.Fatal("jump host key was not checked")
	}
}

func TestSSHHandshakeDeadline(t *testing.T) {
	o, _ := fakeSSH(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.Copy(io.Discard, conn)
	}()
	o.Address = l.Addr().String()
	o.ConnectTimeout = 100 * time.Millisecond
	start := time.Now()
	if _, err := DialSSH(context.Background(), o, testPolicy); err == nil {
		t.Fatal("hanging handshake succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("handshake ignored deadline")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handshake connection leaked")
	}
}

func TestSSHQueuedCancellation(t *testing.T) {
	o, count := fakeSSH(t)
	o.CommandTimeout = time.Second
	s, err := DialSSH(context.Background(), o, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { r, _ := s.Run(ctx, "show wait"); done <- r }()
	deadline := time.Now().Add(time.Second)
	for count.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("first command did not start")
		}
		time.Sleep(time.Millisecond)
	}
	queued, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	if _, err := s.Run(queued, "show version"); err == nil {
		t.Fatal("queued command ignored cancellation")
	}
	cancel()
	select {
	case r := <-done:
		if r.Status != "cancelled" {
			t.Fatal(r.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("active command did not cancel")
	}
	if count.Load() != 1 {
		t.Fatal("queued command reached server")
	}
}
