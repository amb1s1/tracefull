package traceroute

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMacOSContinuationCapture(t *testing.T) {
	data, err := os.ReadFile("testdata/macos-multiple-responders.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := (Parser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hops) != 15 {
		t.Fatalf("got %d hops", len(got.Hops))
	}
	expected := map[int]int{9: 2, 11: 2, 13: 3, 14: 3, 15: 3}
	for _, h := range got.Hops {
		if h.Sent == nil || *h.Sent != 3 {
			t.Errorf("TTL %d: sent %v, want 3", h.TTL, h.Sent)
		}
		want := 1
		if n, ok := expected[h.TTL]; ok {
			want = n
		}
		if len(h.Addresses) != want {
			t.Errorf("TTL %d: %d addresses, want %d", h.TTL, len(h.Addresses), want)
		}
	}
	h := got.Hops[8]
	if math.Abs(*h.Avg-(13.566+14.163+15.271)/3) > 1e-9 {
		t.Fatalf("continuation RTTs missing: %v", *h.Avg)
	}
	if !reflect.DeepEqual(h.Evidence, []string{"line:10", "line:11"}) {
		t.Fatalf("missing source attribution: %v", h.Evidence)
	}
	for _, n := range got.Notes {
		if strings.HasPrefix(n, "Unparsed") {
			t.Fatal(n)
		}
	}
}

// Synthetic edge cases complement the scrubbed user capture.
func TestContinuationBoundaries(t *testing.T) {
	input := "traceroute to dst (2001:db8::9), 30 hops max\n1 2001:db8::1 1 ms\n    other (2001:db8::2) 3 ms * !X\n    MPLS Label 20 TTL=1\n2 2001:db8::9 5 ms\n"
	got, e := (Parser{}).Parse([]byte(input))
	if e != nil {
		t.Fatal(e)
	}
	h := got.Hops[0]
	if *h.Sent != 3 || *h.Lost != 1 || *h.Avg != 2 || len(h.Addresses) != 2 || len(h.Names) != 1 {
		t.Fatalf("bad grouped hop: %+v", h)
	}
	if !reflect.DeepEqual(h.Evidence, []string{"line:2", "line:3"}) {
		t.Fatal(h.Evidence)
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "Unparsed line 4") {
		t.Fatal("extension silently consumed")
	}
}
func TestIPv6NumericContinuation(t *testing.T) {
	got, e := (Parser{}).Parse([]byte("1 2001:db8::1 1 ms\n    2001:db8::2 3 ms\n"))
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Hops) != 1 || *got.Hops[0].Sent != 2 {
		t.Fatalf("IPv6 address mistaken for TTL: %+v", got)
	}
}

func TestTimeoutContinuationAndOrphan(t *testing.T) {
	got, e := (Parser{}).Parse([]byte("    192.0.2.8 1 ms\n1 192.0.2.1 1 ms\n    * *\n2 192.0.2.2 2 ms\n"))
	if e != nil {
		t.Fatal(e)
	}
	if *got.Hops[0].Sent != 3 || *got.Hops[0].Lost != 2 {
		t.Fatal("continuation timeouts were not counted")
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "Unparsed line 1") {
		t.Fatal("orphan responder silently attached")
	}
}

func TestMacOSTraceroute6Header(t *testing.T) {
	// Synthetic IPv6 form emitted by the macOS traceroute6 command.
	input := []byte("traceroute6 to dst (2001:db8::2), 30 hops max, 12 byte packets\n1 2001:db8::1 1 ms\n2 2001:db8::2 2 ms\n")
	if !(Parser{}).Detect(input) {
		t.Fatal("IPv6 header not detected")
	}
	got, err := (Parser{}).Parse(input)
	if err != nil || got.DestAddr != "2001:db8::2" || got.Dest != "dst" {
		t.Fatal(got, err)
	}
	for _, n := range got.Notes {
		if strings.Contains(n, "Unparsed") {
			t.Fatal(n)
		}
	}
}
