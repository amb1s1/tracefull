package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amb1s1/tracefull/internal/device"
)

func TestEndpointAliasesShareSessionGate(t *testing.T) {
	// Synthetic inventory aliases for the same SSH listener. No network traffic.
	for _, pair := range [][2]string{
		{"192.0.2.1", "192.0.2.1:22"},
		{"2001:db8::1", "[2001:0db8:0:0:0:0:0:1]:22"},
		{"192.0.2.1/24", "[::ffff:192.0.2.1]:22"},
		{"Router.EXAMPLE", "router.example:22"},
		{"fe80::1%En0", "[fe80::1%En0]:22"},
		{"192.0.2.1:022", "192.0.2.1:ssh"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			calls := 0
			f := serializeHosts(func(context.Context, DevicePlan, device.Policy) (device.Session, error) {
				calls++
				return &fakeSession{t: t}, nil
			})
			dp := DevicePlan{}
			dp.Device.Address = pair[0]
			first, err := f(context.Background(), dp, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = first.Close() })
			dp.Device.Address = pair[1]
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			second, err := f(ctx, dp, nil)
			if second != nil {
				_ = second.Close()
			}
			if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
				t.Fatalf("alias bypassed active session: calls=%d err=%v", calls, err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}
			second, err = f(context.Background(), dp, nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = second.Close()
		})
	}
}

func TestDistinctEndpointsDoNotShareGate(t *testing.T) {
	for _, pair := range [][2]string{
		{"192.0.2.1", "192.0.2.1:2222"},
		{"fe80::1%En0", "fe80::1%en0"},
	} {
		f := serializeHosts(func(context.Context, DevicePlan, device.Policy) (device.Session, error) {
			return &fakeSession{t: t}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		for _, address := range pair {
			dp := DevicePlan{}
			dp.Device.Address = address
			s, err := f(ctx, dp, nil)
			if err != nil {
				cancel()
				t.Fatalf("distinct endpoints serialized: %q: %v", pair, err)
			}
			t.Cleanup(func() { _ = s.Close() })
		}
		cancel()
	}
}
