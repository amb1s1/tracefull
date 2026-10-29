package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/verdict"
)

func options(t *testing.T) Options {
	return Options{From: "-", UserConfigDir: t.TempDir(), ProjectDir: t.TempDir(), Timeout: time.Second, LatencyJump: verdict.DefaultLatencyJump}
}

const saved = "traceroute to dst (192.0.2.2), 30 hops max\n1 192.0.2.1 1 ms\n2 192.0.2.2 2 ms\n"

func TestPipeline(t *testing.T) {
	o := options(t)
	r, e := Build(context.Background(), o, io.NopCloser(strings.NewReader(saved)), nil)
	if e != nil {
		t.Fatal(e)
	}
	if r.Reached == nil || !*r.Reached || r.Source.Bytes != len(saved) || r.DNSObservedAt != nil {
		t.Fatalf("%+v", r)
	}
	o.Dest = "192.0.2.3"
	if _, e := Build(context.Background(), o, io.NopCloser(strings.NewReader(saved)), nil); e == nil {
		t.Fatal("conflict accepted")
	}
}
func TestInventoryRequired(t *testing.T) {
	o := options(t)
	o.Inventory = filepath.Join(t.TempDir(), "missing")
	if _, e := Build(context.Background(), o, io.NopCloser(strings.NewReader(saved)), nil); e == nil {
		t.Fatal("explicit missing inventory accepted")
	}
}
func TestFileAndOverrides(t *testing.T) {
	o := options(t)
	p := filepath.Join(t.TempDir(), "trace")
	if e := os.WriteFile(p, []byte(saved), 0600); e != nil {
		t.Fatal(e)
	}
	o.From = p
	o.Maps = []string{"garbage"}
	if _, e := Build(context.Background(), o, nil, nil); e == nil {
		t.Fatal("invalid map")
	}
	o.Maps = nil
	if _, e := Build(context.Background(), o, nil, nil); e != nil {
		t.Fatal(e)
	}
}
func TestCancelBlockedInput(t *testing.T) {
	r, w := io.Pipe()
	defer func() { _ = w.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	o := options(t)
	go func() { _, e := Build(ctx, o, r, nil); done <- e }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("read stranded")
	}
}
func TestInputLimit(t *testing.T) {
	o := options(t)
	if _, e := Build(context.Background(), o, io.NopCloser(strings.NewReader(strings.Repeat("x", MaxInputBytes+1))), nil); e == nil {
		t.Fatal("oversized input accepted")
	}
}
