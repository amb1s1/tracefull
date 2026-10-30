package collect

import (
	"context"
	"net"
	"net/netip"
	"strconv"
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
		key := endpointKey(dp.Device.Address)
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

func endpointKey(address string) string {
	if prefix, err := netip.ParsePrefix(address); err == nil {
		address = prefix.Addr().String()
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host, port = address, "22"
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		// Preserve case-sensitive IPv6 interface zones while normalizing the IP.
		host = addr.Unmap().String()
	} else {
		host = strings.ToLower(host)
	}
	if port == "ssh" {
		port = "22"
	} else if n, err := strconv.ParseUint(port, 10, 16); err == nil {
		port = strconv.FormatUint(n, 10)
	}
	return net.JoinHostPort(host, port)
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
