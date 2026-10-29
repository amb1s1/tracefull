// Package app owns the saved-trace pipeline and its resource lifecycle.
package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/muesli/cancelreader"

	"github.com/amb1s1/tracefull/internal/enrich"
	"github.com/amb1s1/tracefull/internal/importer"
	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/report"
	"github.com/amb1s1/tracefull/internal/trace"
	"github.com/amb1s1/tracefull/internal/verdict"
)

const Version = "0.1.0-dev"
const MaxInputBytes = 16 << 20

type Options struct {
	From, Inventory, Dest, UserConfigDir, ProjectDir string
	Maps                                             []string
	RDNS, ASN                                        bool
	Timeout, LatencyJump                             time.Duration
}

// Build keeps the ordered pipeline visible; each blocking stage owns its cleanup.
func Build(ctx context.Context, o Options, stdin io.ReadCloser, resolver enrich.Resolver) (report.Report, error) {
	r := report.Report{}
	if o.From == "" {
		return r, fmt.Errorf("--from is required")
	}
	if o.Timeout <= 0 || o.LatencyJump <= 0 {
		return r, fmt.Errorf("timeouts and latency jump must be positive")
	}
	data, e := read(ctx, o.From, stdin)
	if e != nil {
		return r, e
	}
	t, e := importer.Registry().Parse(data)
	if e != nil {
		return r, e
	}
	if o.Dest != "" {
		a, e := netip.ParseAddr(o.Dest)
		if e != nil {
			return r, fmt.Errorf("--dest must be an IP address: %w", e)
		}
		if t.DestAddr != "" && a.Unmap().String() != t.DestAddr {
			return r, fmt.Errorf("--dest conflicts with saved destination")
		}
		t.DestAddr = a.Unmap().String()
		t.Dest = o.Dest
		t, e = trace.Finish(t)
		if e != nil {
			return r, e
		}
	}
	inv, notes, e := loadInventory(o)
	if e != nil {
		return r, e
	}
	t.Notes = append(t.Notes, notes...)
	over := map[string]string{}
	for _, m := range o.Maps {
		ip, name, ok := strings.Cut(m, "=")
		if !ok {
			return r, fmt.Errorf("--map requires IP=DEVICE")
		}
		a, e := netip.ParseAddr(ip)
		if e != nil {
			return r, e
		}
		key := a.Unmap().String()
		if prev, ok := over[key]; ok && prev != name {
			return r, fmt.Errorf("conflicting map for %s", key)
		}
		over[key] = name
	}
	if e := enrich.Apply(ctx, &t, inv, resolver, enrich.Options{RDNS: o.RDNS, ASN: o.ASN, Timeout: o.Timeout, Overrides: over}); e != nil {
		return r, e
	}
	t.Notes = append(t.Notes, verdict.Notes(t)...)
	r = report.Report{SchemaVersion: "1", Version: Version, Trace: t, Source: report.Source{Name: o.From, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: len(data)}, Findings: verdict.Evaluate(t, o.LatencyJump), Devices: inv.Devices}
	if r.Devices == nil {
		r.Devices = []inventory.Device{}
	}
	if o.RDNS || o.ASN {
		r.DNSObservedAt = trace.Ptr(time.Now().UTC().Format(time.RFC3339Nano))
	}
	return r, nil
}
func read(ctx context.Context, path string, stdin io.ReadCloser) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	r := stdin
	if path != "-" {
		info, e := os.Stat(path)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("input must be a regular file or stdin")
		}
		f, e := os.Open(path)
		if e != nil {
			return nil, e
		}
		r = f
	}
	if r == nil {
		return nil, fmt.Errorf("stdin unavailable")
	}
	b, e := readCancellable(ctx, r)

	if path != "-" {
		if ce := r.Close(); e == nil {
			e = ce
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if e != nil {
		return nil, e
	}
	if len(b) > MaxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", MaxInputBytes)
	}
	return b, nil
}
func loadInventory(o Options) (inventory.Inventory, []string, error) {
	paths := []string{o.Inventory}
	if o.Inventory == "" {
		paths = []string{filepath.Join(o.ProjectDir, "inventory", "devices.yaml"), filepath.Join(o.UserConfigDir, "tracefull", "devices.yaml")}
	}
	for _, p := range paths {
		info, e := os.Stat(p)
		if os.IsNotExist(e) && o.Inventory == "" {
			continue
		}
		if e != nil {
			return inventory.Inventory{}, nil, e
		}
		if !info.Mode().IsRegular() || info.Size() > MaxInputBytes {
			return inventory.Inventory{}, nil, fmt.Errorf("inventory must be a regular file of at most %d bytes", MaxInputBytes)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return inventory.Inventory{}, nil, e
		}
		return inventory.Load(b)
	}
	return inventory.Inventory{}, []string{"No default inventory found; device matching unavailable."}, nil
}
func CheckFindings(r report.Report, kinds []string) bool {
	for _, f := range r.Findings {
		for _, k := range kinds {
			if f.Kind == k {
				return true
			}
		}
	}
	return false
}

// readCancellable uses descriptor polling for Unix stdin: closing a blocking
// os.Stdin alone does not unblock a read on macOS. Regular files and injected
// readers keep their ordinary Close cancellation path.
func readCancellable(ctx context.Context, r io.ReadCloser) (data []byte, err error) {
	reader := io.Reader(r)
	cancel := func() { _ = r.Close() }
	cleanup := func() error { return nil }
	if f, ok := r.(*os.File); ok && runtime.GOOS != "windows" {
		info, e := f.Stat()
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() {
			cr, e := cancelreader.NewReader(f)
			if e != nil {
				return nil, e
			}
			reader = cr
			cancel = func() {
				if !cr.Cancel() {
					_ = r.Close()
				}
			}
			cleanup = cr.Close
		}
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { defer close(closed); cancel() })
	defer func() {
		if !stop() {
			<-closed
		}
		if e := cleanup(); err == nil {
			err = e
		}
	}()
	return io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
}
