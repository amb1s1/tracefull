package collect

import (
	"context"
	"net/netip"
	"strings"
	"sync"

	"github.com/amb1s1/tracefull/internal/device"
)

// Inventory aliases sharing one literal endpoint must not open parallel sessions.
// Different DNS aliases cannot be correlated without resolution; plans do no DNS.
func serializeHosts(factory Factory) Factory {
	var mu sync.Mutex
	gates := map[string]chan struct{}{}
	return func(ctx context.Context, dp DevicePlan, p device.Policy) (device.Session, error) {
		key := strings.ToLower(dp.Device.Address)
		if prefix, e := netip.ParsePrefix(key); e == nil {
			key = prefix.Addr().Unmap().String()
		}
		if addr, e := netip.ParseAddr(key); e == nil {
			key = addr.Unmap().String()
		}
		mu.Lock()
		gate := gates[key]
		if gate == nil {
			gate = make(chan struct{}, 1)
			gates[key] = gate
		}
		mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case gate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		session, err := factory(ctx, dp, p)
		if err != nil {
			<-gate
			return nil, err
		}
		return &hostSession{Session: session, release: func() { <-gate }}, nil
	}
}

type hostSession struct {
	device.Session
	once    sync.Once
	release func()
	err     error
}

func (s *hostSession) Close() error {
	s.once.Do(func() { defer s.release(); s.err = s.Session.Close() })
	return s.err
}
