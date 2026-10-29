package importer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These inputs are synthetic protocol edge cases, not device captures.
var cases = []struct {
	name, input, tool string
	hops              int
}{
	{"traceroute", "traceroute to dst (192.0.2.3), 30 hops max\n 1 edge (192.0.2.1) 1.0 ms 2.0 ms *\n 2 * * *\n 3 192.0.2.3 10 ms 11 ms 12 ms\n", "traceroute", 3},
	{"ipv6", "traceroute to dst (2001:db8::2), 30 hops max\n1 2001:db8::1 1 ms 2001:db8::3 2 ms * !X\n2 2001:db8::2 3 ms\n", "traceroute", 2},
	{"headerless", "1 192.0.2.1 1 ms *\n2 192.0.2.2 2 ms\n", "traceroute", 2},
	{"windows", "Tracing route to dst [192.0.2.2]\nover a maximum of 30 hops:\n1 <1 ms <1 ms <1 ms 192.0.2.1\n2 2 ms 3 ms 4 ms dst [192.0.2.2]\nTrace complete.\n", "tracert", 2},
	{"windows-timeout", "Tracing route to 192.0.2.2\nover a maximum of 30 hops:\n1 * * * Request timed out.\nTrace complete.\n", "tracert", 1},
	{"tracepath", " 1?: [LOCALHOST] pmtu 1500\n 1: 192.0.2.1 1.0ms\n 2: no reply\n 3: 192.0.2.3 3.0ms pmtu 1400 asymm 2\n 3: 192.0.2.3 4.0ms reached\n Resume: pmtu 1400 hops 3 back 2\n", "tracepath", 3},
	{"mtr", "Start: 2026-01-01\nHOST: example Loss% Snt Last Avg Best Wrst StDev\n1.|-- 192.0.2.1 10.0% 100 2 2 1 3 1\n2.|-- 192.0.2.2 0.0% 100 2 2 1 3 1\n", "mtr", 2},
	{"mtr-wide", "HOST: example Loss% Snt Last Avg Best Wrst StDev\n1. AS64500 router.example (192.0.2.1) 0.0% 100 2 2 1 3 1\n 192.0.2.4\n2. AS??? ??? 100.0% 100 0 0 0 0 0\n", "mtr", 2},
	{"mtr-json", `{"report":{"mtr":{"dst":"192.0.2.2","src":"192.0.2.10","tests":100},"hubs":[{"count":1,"host":"192.0.2.1","Loss%":10,"Snt":100,"Avg":2,"Best":1,"Wrst":3,"StDev":1},{"count":2,"host":"192.0.2.2","Loss%":0,"Snt":100,"Avg":3,"Best":2,"Wrst":4}]}}`, "mtr", 2},
	{"mtr-unknown", `{"report":{"mtr":{"dst":"dst.example"},"hubs":[{"count":1,"host":"router.example","Snt":null}]}}`, "mtr", 1},
	{"paris", "traceroute [(192.0.2.10:33456) -> (192.0.2.2:33457)], protocol udp, algo exhaustive, duration 1 s\n1 P(6, 6) 192.0.2.1 (192.0.2.1) 1.0/2.0/3.0/1.0 ms\n2 P(6, 6) 192.0.2.2 (192.0.2.2) 2.0/3.0/4.0/1.0 ms\n", "paris", 2},
	{"dublin", `{"flows":{"33434":[{"sent":{"ip":{"src":"192.0.2.10","dst":"192.0.2.2","ttl":1}},"received":{"ip":{"src":"192.0.2.1"},"icmp":{"type":11,"code":0}},"rtt_usec":1000,"nat_id":0},{"sent":{"ip":{"src":"192.0.2.10","dst":"192.0.2.2","ttl":2}},"received":{"ip":{"src":"192.0.2.2"},"icmp":{"type":3,"code":3}},"rtt_usec":2000,"nat_id":1,"is_last":true}]}}`, "dublin", 2},
}

func TestDialects(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, e := Registry().Parse([]byte(tc.input))
			if e != nil {
				t.Fatal(e)
			}
			if got.Probe.Tool != tc.tool || len(got.Hops) != tc.hops {
				t.Fatalf("wrong parse: %+v", got)
			}
			again, e := Registry().Parse([]byte(tc.input))
			if e != nil || !reflect.DeepEqual(got, again) {
				t.Fatal("not deterministic")
			}
			if _, e := json.Marshal(got); e != nil {
				t.Fatal(e)
			}
			if tc.name == "windows" && got.Hops[0].Avg != nil {
				t.Fatal("censored RTT became exact")
			}
			if tc.name == "mtr-wide" && got.Hops[1].Avg != nil {
				t.Fatal("100% loss has RTT")
			}
			if tc.name == "paris" && got.Hops[0].Sent != nil {
				t.Fatal("fabricated sample count")
			}
			if tc.name == "traceroute" && (*got.Hops[0].Sent != 3 || *got.Hops[0].Lost != 1) {
				t.Fatal("bad sample accounting")
			}
		})
	}
}
func TestBadInputs(t *testing.T) {
	for _, s := range []string{"", "garbage", `{"report":`, "traceroute to x\n1 garbage", "1 192.0.2.1 1 ms\n1 192.0.2.2 2 ms", "256 192.0.2.1 1 ms", `{"report":{"hubs":[{"count":1,"Snt":-1}]}}`, `{"report":{"hubs":[{"count":1,"Loss%":101}]}}`, `{"report":{"hubs":[{"count":1,"Snt":1.5}]}}`, `{"report":{"hubs":[{"count":1,"Avg":1,"Best":2,"Wrst":3}]}}`, `{"flows":{"1":[{"sent":{"ip":{"dst":"bad","ttl":1}}}]}}`, "HOST: x Loss% Snt\n1.|-- broken", "HOST: x Loss% Snt\n1.|-- 192.0.2.1 abc% 100"} {
		if _, e := Registry().Parse([]byte(s)); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
func TestTruncatedUnknown(t *testing.T) {
	got, e := Registry().Parse([]byte("traceroute to dst (192.0.2.2), 30 hops max\n1 192.0.2.1 1 ms\n"))
	if e != nil {
		t.Fatal(e)
	}
	if got.Reached != nil {
		t.Fatal("truncation became unreachable")
	}
	if got.StartedAt != nil {
		t.Fatal("invented measurement time")
	}
}
func TestDublinUpstream(t *testing.T) {
	b, e := os.ReadFile("../../testdata/dublin-upstream.json")
	if e != nil {
		t.Fatal(e)
	}
	got, e := Registry().Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Paths) != 1 || len(got.Hops) == 0 {
		t.Fatal("missing flow")
	}
}
func FuzzParse(f *testing.F) {
	for _, tc := range cases {
		f.Add([]byte(tc.input))
	}
	capture, e := os.ReadFile("../parse/traceroute/testdata/macos-multiple-responders.txt")
	if e != nil {
		f.Fatal(e)
	}
	f.Add(capture)
	f.Add([]byte("garbage"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		a, e := Registry().Parse(b)
		c, e2 := Registry().Parse(b)
		if (e == nil) != (e2 == nil) || !reflect.DeepEqual(a, c) {
			t.Fatal("nondeterministic")
		}
		if e == nil {
			if len(a.Hops) == 0 {
				t.Fatal("empty success")
			}
			if _, e := json.Marshal(a); e != nil {
				t.Fatal(e)
			}
			for _, h := range a.Hops {
				for _, ip := range h.Addresses {
					if strings.ContainsAny(ip, "\n\r\x1b") {
						t.Fatal("unsafe address")
					}
				}
			}
		}
	})
}

func TestMetadataRegressions(t *testing.T) {
	for _, tc := range cases {
		got, e := Registry().Parse([]byte(tc.input))
		if e != nil {
			t.Fatal(e)
		}
		if tc.name == "tracepath" {
			if got.Hops[0].PMTU == nil || *got.Hops[0].PMTU != 1500 || len(got.Hops[2].Addresses) != 1 {
				t.Fatal("local MTU or retry address lost")
			}
		}
	}
	data := []byte(`{"report":{"hubs":[{"count":1,"host":"leaf (192.0.2.1)","addr":"192.0.2.1","Snt":0,"Loss%":0,"Avg":0}]}}`)
	got, e := Registry().Parse(data)
	if e != nil {
		t.Fatal(e)
	}
	h := got.Hops[0]
	if len(h.Addresses) != 1 || len(h.Names) != 1 || h.Avg != nil || h.Loss != nil {
		t.Fatalf("bad empty sample or duplicate host: %+v", h)
	}
}

func TestParserGoldens(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, e := Registry().Parse([]byte(tc.input))
			if e != nil {
				t.Fatal(e)
			}
			encoded, e := json.MarshalIndent(got, "", "  ")
			if e != nil {
				t.Fatal(e)
			}
			encoded = append(encoded, '\n')
			path := filepath.Join("testdata", tc.name+".json")
			if os.Getenv("TRACEFULL_UPDATE_GOLDENS") == "1" {
				if e := os.WriteFile(path, encoded, 0600); e != nil {
					t.Fatal(e)
				}
			}
			want, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(encoded, want) {
				t.Fatalf("parsed data changed: review %s before updating", path)
			}
		})
	}
}
