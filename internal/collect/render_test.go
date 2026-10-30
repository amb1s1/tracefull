package collect

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/evidence"
)

func TestRenderEvidenceStreams(t *testing.T) {
	// Synthetic failures cover partially saved evidence as well as two-stream capture.
	for _, tc := range []struct {
		name           string
		stdout, stderr *evidence.File
		want           string
	}{
		{"both", &evidence.File{Path: "read.stdout"}, &evidence.File{Path: "read.stderr"}, " [stdout: read.stdout] [stderr: read.stderr]"},
		{"stderr only", nil, &evidence.File{Path: "read.stderr"}, " [stderr: read.stderr]"},
		{"stdout only", &evidence.File{Path: "read.stdout"}, nil, " [stdout: read.stdout]"},
		{"neither", nil, nil, ""},
	} {
		for _, format := range []string{"text", "markdown"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				r := Report{Devices: []DeviceResult{{Commands: []CommandResult{{PlannedCommand: PlannedCommand{Command: driver.Command{Name: "optics-eth0", Status: "failed", Reason: "exit 1"}}, Stdout: tc.stdout, Stderr: tc.stderr}}}}}
				var b strings.Builder
				if err := Render(&b, r, format); err != nil {
					t.Fatal(err)
				}
				want := "- optics-eth0: failed — exit 1" + tc.want + "\n"
				if !strings.Contains(b.String(), want) {
					t.Fatalf("missing %q in %s", want, b.String())
				}
			})
		}
	}
}

func TestRenderRouteMetadata(t *testing.T) {
	// Synthetic facts distinguish missing gateways from reported next hops.
	empty, mgmt := "", "mgmt"
	f := driver.Facts{
		Addresses: []driver.Address{{Address: "192.0.2.1"}, {Address: "192.0.2.2", Interface: "eth0", Master: "mgmt", RoutingContext: &mgmt}, {Address: "192.0.2.3", RoutingContext: &empty}},
		Routes: []driver.Route{
			{Prefix: "192.0.2.0/24", Protocol: "bgp", Context: "default", NextHops: []driver.NextHop{{Address: "192.0.2.254", Interface: "swp1"}}},
			{Prefix: "192.0.2.2/32", Protocol: "local", Context: "mgmt", NextHops: []driver.NextHop{{Interface: "eth0"}}},
			{Prefix: "0.0.0.0/0", NextHops: []driver.NextHop{{}}},
		},
	}
	r := Report{Devices: []DeviceResult{{Commands: []CommandResult{{Facts: &f}}}}}
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"text", "markdown"} {
		var b strings.Builder
		if err := Render(&b, r, format); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"Address 192.0.2.1 on unknown (master: unknown; routing context: unknown)",
			"Address 192.0.2.2 on eth0 (master: mgmt; routing context: mgmt)",
			"Address 192.0.2.3 on unknown (master: unknown; routing context: unknown)",
			"Route 192.0.2.0/24 | protocol: bgp | context: default",
			"Route 192.0.2.2/32 | protocol: local | context: mgmt",
			"Route 0.0.0.0/0 | protocol: unknown | context: unknown",
			"Next hop 192.0.2.254 on swp1 | active: ? | installed: ?",
			"Next hop unknown on eth0 | active: ? | installed: ?",
			"Next hop unknown on unknown | active: ? | installed: ?",
		} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("%s: missing %q in %s", format, want, b.String())
			}
		}
	}
	var b strings.Builder
	if err := Render(&b, r, "json"); err != nil {
		t.Fatal(err)
	}
	var after Report
	if err := json.Unmarshal([]byte(b.String()), &after); err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(actual) {
		t.Fatal("human rendering changed report or JSON values")
	}
}

func TestRenderUntrustedEvidenceAndMetadata(t *testing.T) {
	hostile := "<tag>*[path]\x1b\n"
	r := Report{Devices: []DeviceResult{{Commands: []CommandResult{{
		Stderr: &evidence.File{Path: hostile},
		Facts:  &driver.Facts{Addresses: []driver.Address{{Master: hostile}}, Routes: []driver.Route{{Protocol: hostile, Context: hostile, NextHops: []driver.NextHop{{Address: hostile, Interface: hostile}}}}},
	}}}}}
	for _, tc := range []struct{ format, safe string }{
		{"text", "<tag>*[path]\\u001b\\u000a"},
		{"markdown", "&lt;tag&gt;&#42;&#91;path&#93;&#92;u001b&#92;u000a"},
	} {
		var b strings.Builder
		if err := Render(&b, r, tc.format); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(b.String(), "\x1b") || strings.Contains(b.String(), hostile) || strings.Count(b.String(), tc.safe) != 6 {
			t.Fatalf("%s: unsafe or missing evidence/metadata: %s", tc.format, b.String())
		}
	}
}
