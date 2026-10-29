package enrich

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/trace"
)

type fake struct {
	calls int
	block bool
	fail  bool
}

func (f *fake) LookupAddr(ctx context.Context, s string) ([]string, error) {
	f.calls++
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.fail {
		return nil, errors.New("failed")
	}
	return []string{"leaf."}, nil
}
func (f *fake) LookupTXT(ctx context.Context, s string) ([]string, error) {
	f.calls++
	return []string{"15169 | 8.8.8.0/24 | US | arin | date"}, nil
}
func TestDNS(t *testing.T) {
	h := trace.NewHop(1, "line:1")
	h.AddAddress("8.8.8.8")
	v := trace.Trace{Hops: []trace.Hop{h, h}}
	inv := inventory.Inventory{Devices: []inventory.Device{{Name: "leaf"}}}
	f := &fake{}
	if e := Apply(context.Background(), &v, inv, f, Options{RDNS: true, ASN: true, Timeout: time.Second}); e != nil {
		t.Fatal(e)
	}
	if f.calls != 2 || v.Hops[0].Matches["8.8.8.8"].Device != "leaf" || v.Hops[0].ASN["8.8.8.8"][0] != "15169" {
		t.Fatal("lookup cache or attribution failed")
	}
}
func TestCancellation(t *testing.T) {
	h := trace.NewHop(1, "line:1")
	h.AddAddress("8.8.8.8")
	v := trace.Trace{Hops: []trace.Hop{h}}
	ctx, cancel := context.WithCancel(context.Background())
	f := &fake{block: true}
	done := make(chan error, 1)
	go func() { done <- Apply(ctx, &v, inventory.Inventory{}, f, Options{RDNS: true, Timeout: time.Hour}) }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("lookup stranded")
	}
}
func TestOfflineAndTimeout(t *testing.T) {
	h := trace.NewHop(1, "line:1")
	h.AddAddress("10.0.0.1")
	v := trace.Trace{Hops: []trace.Hop{h}}
	f := &fake{}
	if e := Apply(context.Background(), &v, inventory.Inventory{}, f, Options{Timeout: time.Second}); e != nil || f.calls != 0 {
		t.Fatal("offline lookup")
	}
	f.block = true
	if e := Apply(context.Background(), &v, inventory.Inventory{}, f, Options{RDNS: true, Timeout: time.Millisecond}); e != nil {
		t.Fatal(e)
	}
	if len(v.Hops[0].Notes) == 0 {
		t.Fatal("failure hidden")
	}
}
func TestASNQuery(t *testing.T) {
	for _, s := range []string{"bad", "10.0.0.1", "127.0.0.1", "fe80::1", "2001:db8::1", "192.0.2.1", "100.64.0.1", "255.255.255.255"} {
		if ASNQuery(s) != "" {
			t.Fatal(s)
		}
	}
	if ASNQuery("8.8.4.4") != "4.4.8.8.origin.asn.cymru.com" {
		t.Fatal("v4 query")
	}
	if !strings.HasSuffix(ASNQuery("2001:4860:4860::8888"), ".origin6.asn.cymru.com") {
		t.Fatal("v6 query")
	}
}
