package collect

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/amb1s1/tracefull/internal/device"
)

func TestRouterOSCapturePipeline(t *testing.T) {
	p := prepare(t, nil)
	p.Options.Groups = []string{"route", "version", "cpu", "counters", "optics", "lldp"}
	p.Plan.Destination = "1.1.1.1"
	d := p.Plan.Devices[0].Device
	d.Platform = "routeros"
	dp, e := devicePlan(p, d, []string{}, false)
	if e != nil {
		t.Fatal(e)
	}
	p.Plan.Devices = []DevicePlan{dp}
	wrap := func(s string) string {
		return ":put [:serialize to=json options=json.no-string-conversion value=[" + s + "]]"
	}
	s := &captureSession{t: t, dialect: "routeros", files: map[string]string{
		"/system resource print":           "resource.txt",
		wrap("/ip address print as-value"): "addresses4.json", wrap("/ipv6 address print as-value"): "addresses6.json",
		wrap("/ip route print detail as-value where 1.1.1.1 in dst-address and routing-table=main"): "route4.json",
		wrap("/ip neighbor print detail as-value"):                                                  "neighbors.json",
		":put [:serialize to=json options=json.no-string-conversion value=([/interface print detail as-value where name=sfp28-1],[/interface print stats-detail as-value where name=sfp28-1])]": "counters.json",
		"/interface ethernet monitor sfp28-1 once": "optics.txt",
	}}
	r, e := Run(context.Background(), p, func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
		s.policy = policy
		return s, nil
	})
	if e != nil || r.Status != "complete" || !s.closed {
		t.Fatal(r.Status, e, s.closed)
	}
	if !slices.Equal(r.Devices[0].Interfaces, []string{"sfp28-1"}) {
		t.Fatal(r.Devices[0].Interfaces)
	}
	counters, optics := 0, 0
	for _, c := range r.Devices[0].Commands {
		if c.Status != "ok" || c.Facts == nil {
			t.Fatal(c)
		}
		counters += len(c.Facts.Counters)
		optics += len(c.Facts.Optics)
	}
	if counters != 1 || optics != 1 {
		t.Fatal(counters, optics)
	}
	validateReport(t, r)
	// Only the explicit active IPv6 BGP row may drive interface discovery.
	drv, _ := Registry("routeros")
	f, e := drv.Parse("route-2606:4700:4700::1111", func() []byte {
		b, e := os.ReadFile("../driver/routeros/testdata/route6.json")
		if e != nil {
			t.Fatal(e)
		}
		return b
	}())
	if e != nil {
		t.Fatal(e)
	}
	got := interfaces(nil, []CommandResult{{Facts: &f}}, "default")
	if !slices.Equal(got, []string{"wg-peer1"}) {
		t.Fatal(got)
	}
	for _, route := range f.Routes {
		if route.Installed != nil || comparableRoute(route, "2606:4700:4700::1111", "default") {
			t.Fatal("invented hardware state", route)
		}
	}
}
