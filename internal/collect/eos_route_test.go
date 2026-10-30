package collect

import (
	"context"
	"slices"
	"testing"

	"github.com/amb1s1/tracefull/internal/device"
)

func TestEOSRouteDiscovery(t *testing.T) {
	for _, tc := range []struct {
		dest, command, fixture string
		interfaces             []string
		routes                 int
	}{
		{"1.1.1.1", "show ip route 1.1.1.1 | json", "route4-empty.json", []string{}, 0},
		{"2001:db8::1", "show ipv6 route 2001:db8::1 | json", "route6.json", []string{"Vlan2602"}, 1},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			p := prepare(t, nil)
			p.Options.Groups = []string{"route"}
			p.Plan.Destination = tc.dest
			d := p.Plan.Devices[0].Device
			d.Platform = "eos"
			dp, err := devicePlan(p, d, []string{}, false)
			if err != nil {
				t.Fatal(err)
			}
			p.Plan.Devices = []DevicePlan{dp}
			s := &captureSession{t: t, dialect: "eos", files: map[string]string{tc.command: tc.fixture}}
			r, err := Run(context.Background(), p, func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
				s.policy = policy
				return s, nil
			})
			if err != nil || r.Status != "complete" || !s.closed {
				t.Fatal(r.Status, err, s.closed)
			}
			dev := r.Devices[0]
			if !slices.Equal(dev.Interfaces, tc.interfaces) || len(dev.Commands) != 1 || dev.Commands[0].Facts == nil || len(dev.Commands[0].Facts.Routes) != tc.routes {
				t.Fatal("incorrect route-driven discovery", dev)
			}
			validateReport(t, r)
		})
	}
}
