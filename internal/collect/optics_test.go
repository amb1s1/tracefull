package collect

import (
	"context"
	"strings"
	"testing"
)

func TestOpticsCollectionPreservesUnavailableInterface(t *testing.T) {
	// A synthetic session replays the OSFP format for discovered switch ports.
	// The real capture exists only for swp49; this does not claim eight live captures.
	p := prepare(t, func(o *Options) { o.Groups = []string{"optics"} })
	for _, c := range p.Plan.Devices[0].Commands {
		if c.Group == "optics" && c.Status != "deferred" {
			t.Fatal(c)
		}
	}
	s := &fakeSession{t: t, fail: "sudo -n ethtool -m eth0"}
	r, err := Run(context.Background(), p, factory(s))
	if err != nil || r.Status != "partial" || !s.closed {
		t.Fatal(r.Status, err)
	}
	validateReport(t, r)
	parsed, failed := 0, 0
	for _, c := range r.Devices[0].Commands {
		if c.Group != "optics" {
			continue
		}
		if c.Name == "optics-eth0" {
			failed++
			if c.Status != "failed" || c.Facts != nil || c.Stderr == nil {
				t.Fatal(c)
			}
			continue
		}
		parsed++
		if c.Status != "ok" || c.Facts == nil || len(c.Facts.Optics) != 8 {
			t.Fatal(c)
		}
		for _, o := range c.Facts.Optics {
			if c.Name != "optics-"+o.Interface {
				t.Fatal(o)
			}
		}
	}
	if parsed != 8 || failed != 1 {
		t.Fatal(parsed, failed)
	}
	attempts := 0
	for _, cmd := range s.commands {
		if strings.Contains(cmd, "ethtool -m eth0") {
			attempts++
		}
	}
	if attempts != 1 {
		t.Fatal("retried unavailable read")
	}
}
