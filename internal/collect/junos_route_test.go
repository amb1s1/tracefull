package collect

import (
	"context"
	"slices"
	"testing"

	"github.com/amb1s1/tracefull/internal/device"
)

func TestJunosISISDiscovery(t *testing.T) {
	p := prepare(t, nil)
	p.Options.Groups = []string{"route"}
	p.Plan.Destination = "192.0.2.10"
	d := p.Plan.Devices[0].Device
	d.Platform = "junos"
	dp, err := devicePlan(p, d, []string{}, false)
	if err != nil {
		t.Fatal(err)
	}
	p.Plan.Devices = []DevicePlan{dp}
	s := &captureSession{t: t, dialect: "junos", files: map[string]string{"show route table inet.0 192.0.2.10 detail | display json": "route4-isis-detail.json"}}
	r, err := Run(context.Background(), p, func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
		s.policy = policy
		return s, nil
	})
	if err != nil || r.Status != "complete" || !s.closed {
		t.Fatal(r.Status, err, s.closed)
	}
	if !slices.Equal(r.Devices[0].Interfaces, []string{"et-0/0/0", "et-0/0/0.0"}) {
		t.Fatal("unselected next hop drove discovery", r.Devices[0].Interfaces)
	}
	route := r.Devices[0].Commands[0].Facts.Routes[0]
	if len(route.NextHops) != 2 || route.NextHops[1].Active != nil || comparableRoute(route, p.Plan.Destination, "default") {
		t.Fatal("lost unknown next hop or invented hardware state", route)
	}
	validateReport(t, r)
}
