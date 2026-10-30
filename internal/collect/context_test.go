package collect

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestEgressDiscoveryRequiresRequestedContext(t *testing.T) {
	// Synthetic mixed/unknown-context results must not select unrelated egress.
	yes := true
	facts := driver.Empty()
	for _, context := range []string{"default", "mgmt", ""} {
		facts.Routes = append(facts.Routes, driver.Route{
			Context: context, Selected: &yes, Installed: &yes,
			NextHops: []driver.NextHop{{Interface: "port-" + context, Active: &yes, Installed: &yes}},
		})
	}
	// Ownership observations remain useful even when the route context differs.
	facts.Addresses = []driver.Address{{Address: "192.0.2.1", Interface: "ingress"}}
	commands := []CommandResult{{Facts: &facts}}
	for _, context := range []string{"default", "mgmt"} {
		got := interfaces([]string{"192.0.2.1"}, commands, context)
		want := []string{"ingress", "port-" + context}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("context %s: got %v, want %v", context, got, want)
		}
	}
}

func TestManagementCollectionPipeline(t *testing.T) {
	base := prepare(t, nil)
	base.Options.VRF = "mgmt"
	// Synthetic saved trace targets the address represented by the scrubbed route.
	p, err := Prepare(context.Background(), base.Options, io.NopCloser(strings.NewReader("traceroute to 2001:db8:2::3 (2001:db8:2::3), 30 hops max\n1 2001:db8:2::3 1 ms 2 ms 3 ms\n")))
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSession{t: t}
	r, err := Run(context.Background(), p, factory(s))
	if err != nil || r.Status != "complete" || !s.closed {
		t.Fatal(r.Status, err)
	}
	validateReport(t, r)
	d := r.Devices[0]
	if !reflect.DeepEqual(d.Interfaces, []string{"eth0"}) {
		t.Fatal(d.Interfaces)
	}
	route := false
	for _, c := range d.Commands {
		if c.Status != "ok" {
			t.Fatal(c)
		}
		if c.Name == "route-mgmt" {
			route = true
			if len(c.Facts.Routes) != 1 || c.Facts.Routes[0].Context != "mgmt" {
				t.Fatal(c.Facts)
			}
		}
		if c.Group == "counters" && (len(c.Facts.Counters) != 1 || c.Facts.Counters[0].Interface != "eth0") {
			t.Fatal(c.Facts)
		}
		if c.Group == "lldp" {
			for _, n := range c.Facts.Neighbors {
				if n.Interface != "eth0" {
					t.Fatal(n)
				}
			}
		}
	}
	if !route {
		t.Fatal("missing management route")
	}
	for _, f := range r.Findings {
		if f.Kind == "interface-context" || strings.Contains(f.Kind, "mismatch") {
			t.Fatal(f)
		}
	}
}

func TestManagementLookupFailureDoesNotRetryDefault(t *testing.T) {
	p := prepare(t, func(o *Options) { o.VRF = "mgmt" })
	command := "sudo -n vtysh -c 'show ipv6 route vrf mgmt 2001:db8::ffff json'"
	for _, malformed := range []bool{false, true} {
		// Synthetic transport/parse failures preserve raw evidence without a fallback.
		s := &fakeSession{t: t, fail: command}
		if malformed {
			s.fail = ""
			s.malformed = command
		}
		p.Options.EvidenceDir = t.TempDir() + "/evidence"
		r, err := Run(context.Background(), p, factory(s))
		if err != nil || r.Status != "partial" || !s.closed {
			t.Fatal(r.Status, err)
		}
		attempts := 0
		for _, cmd := range s.commands {
			if strings.Contains(cmd, " route ") {
				attempts++
				if cmd != command {
					t.Fatal("context fallback", cmd)
				}
			}
		}
		if attempts != 1 {
			t.Fatal("retried lookup", s.commands)
		}
		for _, c := range r.Devices[0].Commands {
			if c.Name == "route-mgmt" && (c.Facts != nil || c.Stdout == nil || c.Stderr == nil || c.Status == "ok") {
				t.Fatal(c)
			}
		}
		validateReport(t, r)
	}
}
