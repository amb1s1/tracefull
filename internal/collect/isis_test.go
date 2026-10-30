package collect

import (
	"context"
	"slices"
	"testing"

	"github.com/amb1s1/tracefull/internal/device"
)

func TestISISRouteDiscovery(t *testing.T) {
	for _, tc := range []struct {
		dest, family, file string
		ports              int
	}{
		{"192.0.2.69", "ipv4", "route4-isis-single.txt", 1},
		{"192.0.2.70", "ipv4", "route4-isis-multipath.txt", 2},
		{"2001:db8:30::32", "ipv6", "route6-isis-multipath.txt", 2},
	} {
		t.Run(tc.file, func(t *testing.T) {
			p := prepare(t, nil)
			p.Options.Groups = []string{"route"}
			p.Plan.Destination = tc.dest
			d := p.Plan.Devices[0].Device
			d.Platform = "cisco-ios-xr"
			dp, err := devicePlan(p, d, []string{}, false)
			if err != nil {
				t.Fatal(err)
			}
			p.Plan.Devices = []DevicePlan{dp}
			s := &captureSession{t: t, dialect: "iosxr", files: map[string]string{"show route " + tc.family + " " + tc.dest + " detail": tc.file}}
			r, err := Run(context.Background(), p, func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
				s.policy = policy
				return s, nil
			})
			if err != nil || r.Status != "complete" || !s.closed {
				t.Fatal(r.Status, err, s.closed)
			}
			want := []string{"FourHundredGigE0/0/0/4"}
			if tc.ports == 2 {
				want = append(want, "FourHundredGigE0/0/0/5")
			}
			if !slices.Equal(r.Devices[0].Interfaces, want) {
				t.Fatal(r.Devices[0].Interfaces)
			}
			route := r.Devices[0].Commands[0].Facts.Routes[0]
			if comparableRoute(route, tc.dest, "default") {
				t.Fatal("RIB-only evidence became a forwarding comparison")
			}
			validateReport(t, r)
		})
	}
}
