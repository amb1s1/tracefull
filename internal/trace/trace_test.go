package trace

import (
	"math"
	"testing"
)

func TestSamples(t *testing.T) {
	h := NewHop(1, "line:1")
	h.Samples([]float64{1, 2, 3}, 1)
	if *h.Sent != 4 || *h.Lost != 1 || *h.Loss != 25 || *h.Avg != 2 || *h.Best != 1 || *h.Worst != 3 {
		t.Fatalf("%+v", h)
	}
	h = NewHop(1, "line:1")
	h.Samples(nil, 3)
	if h.Avg != nil || *h.Loss != 100 {
		t.Fatal("missing RTT became zero")
	}
	h.Samples(nil, 0)
}
func TestFinish(t *testing.T) {
	h := NewHop(2, "line:2")
	h.AddAddress("192.0.2.2")
	h.AddAddress("192.0.2.2")
	if h.AddAddress("bad") {
		t.Fatal("bad address")
	}
	v, e := Finish(Trace{Hops: []Hop{h}, DestAddr: "192.0.2.2"})
	if e != nil || v.Reached == nil || !*v.Reached || len(v.Notes) == 0 || len(v.Hops[0].Addresses) != 1 {
		t.Fatalf("%+v %v", v, e)
	}
	v, e = Finish(Trace{Hops: []Hop{h}, DestAddr: "192.0.2.3", Complete: true})
	if e != nil || v.Reached == nil || *v.Reached {
		t.Fatal("completed missing destination")
	}
}
func TestBadMeasurements(t *testing.T) {
	for _, edit := range []func(*Hop){func(h *Hop) { h.TTL = 0 }, func(h *Hop) { h.Sent = Ptr(-1) }, func(h *Hop) { h.Lost = Ptr(1) }, func(h *Hop) { h.Sent = Ptr(1); h.Lost = Ptr(2) }, func(h *Hop) { h.Avg = Ptr(math.Inf(1)) }, func(h *Hop) { h.Avg = Ptr(-1.0) }, func(h *Hop) { h.Loss = Ptr(101.0) }, func(h *Hop) { h.Best = Ptr(4.0); h.Avg = Ptr(3.0); h.Worst = Ptr(5.0) }} {
		h := NewHop(1, "line:1")
		edit(&h)
		if _, e := Finish(Trace{Hops: []Hop{h}}); e == nil {
			t.Fatal("bad measurement accepted")
		}
	}
	if _, e := Finish(Trace{}); e == nil {
		t.Fatal("empty trace accepted")
	}
}

func TestUnresolvedDestinationIdentity(t *testing.T) {
	h := NewHop(1, "line:1")
	h.Names = []string{"dest.example"}
	v, e := Finish(Trace{Hops: []Hop{h}, DestAddr: "192.0.2.1", Complete: true})
	if e != nil || v.Reached != nil {
		t.Fatal("hostname-only row cannot prove no destination response")
	}
}

func TestAddressZoneControls(t *testing.T) {
	// netip accepts zones containing controls; normalization must reject them in
	// aggregate hops, individual flows and source/destination identity alike.
	h := NewHop(1, "line:1")
	h.AddAddress("fe80::1%en0")
	for _, change := range []func(*Trace){
		func(v *Trace) { v.Hops[0].Addresses = []string{"::%\x1b"} },
		func(v *Trace) { v.Paths = []Path{{Hops: []Hop{{Addresses: []string{"fe80::1%\n"}}}}} },
		func(v *Trace) { v.DestAddr = "::1%\x00" },
		func(v *Trace) { v.SourceAddr = "::1%\r" },
	} {
		v := Trace{Hops: []Hop{h}}
		change(&v)
		if _, err := Finish(v); err == nil {
			t.Fatal("control zone accepted")
		}
	}
	if _, err := Finish(Trace{Hops: []Hop{h}}); err != nil {
		t.Fatal("safe interface zone rejected", err)
	}
}
