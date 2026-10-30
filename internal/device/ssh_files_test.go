package device

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSSHSymlinkedFiles(t *testing.T) {
	o, _ := fakeSSH(t)
	dir := t.TempDir()
	key, hosts := filepath.Join(dir, "identity"), filepath.Join(dir, "known_hosts")
	if err := os.Symlink(o.IdentityFiles[0], key); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(o.KnownHosts, hosts); err != nil {
		t.Fatal(err)
	}
	o.IdentityFiles, o.KnownHosts = []string{key}, hosts
	s, err := DialSSH(context.Background(), o, testPolicy)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func TestSSHRejectsNamedPipeFiles(t *testing.T) {
	for _, kind := range []string{"known_hosts", "identity"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), kind)
			if err := unix.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			// Hold a writer open so an accidental read blocks. Closing it below
			// releases the reader even when testing the unfixed implementation.
			fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal(err)
			}
			o := SSHOptions{Address: "127.0.0.1:1", User: "test", ConnectTimeout: 20 * time.Millisecond}
			if kind == "known_hosts" {
				o.KnownHosts = path
			} else {
				o.InsecureHostKey = true
				o.IdentityFiles = []string{path}
			}
			done := make(chan error, 1)
			go func() {
				s, err := DialSSH(context.Background(), o, testPolicy)
				if s != nil {
					_ = s.Close()
				}
				done <- err
			}()
			select {
			case err = <-done:
				_ = unix.Close(fd)
				if err == nil || !strings.Contains(err.Error(), "regular file") {
					t.Fatalf("expected explicit non-regular-file rejection, got %v", err)
				}
			case <-time.After(200 * time.Millisecond):
				_ = unix.Close(fd)
				select {
				case err := <-done:
					if err == nil {
						t.Fatal("named pipe accepted as SSH configuration")
					}
				case <-time.After(time.Second):
					t.Fatal("SSH setup did not finish after releasing pipe")
				}
				t.Fatal("SSH file read ignored connection deadline")
			}
		})
	}
}
