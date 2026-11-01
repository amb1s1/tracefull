package iosxe

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

var protocolRE = regexp.MustCompile(`^Known via "([^"]+)"`)

func route(lines []string, f *driver.Facts) error {
	r := driver.Route{Context: "default", NextHops: []driver.NextHop{}}
	paths, noRoute := false, false
	for i, l := range lines {
		s := strings.TrimSpace(l)
		if s == "" {
			continue
		}
		if s == "% Network not in table" {
			noRoute = true
			continue
		}
		if p, ok := strings.CutPrefix(s, "Routing entry for "); ok {
			if r.Prefix != "" {
				return fmt.Errorf("multiple route entries")
			}
			prefix, e := netip.ParsePrefix(p)
			if e != nil || !prefix.Addr().Is6() {
				return fmt.Errorf("only captured IPv6 route entries are validated")
			}
			r.Prefix = prefix.String()
			r.Evidence = ref(i)
			continue
		}
		if m := protocolRE.FindStringSubmatch(s); m != nil {
			if r.Protocol != "" {
				return fmt.Errorf("duplicate route protocol")
			}
			r.Protocol = m[1]
			continue
		}
		if s == "Routing paths:" {
			if paths {
				return fmt.Errorf("duplicate routing paths")
			}
			paths = true
			continue
		}
		if strings.HasPrefix(s, "Route count is ") || strings.HasPrefix(s, "Route metric is ") || strings.HasPrefix(s, "Last updated ") {
			continue
		}
		if !paths {
			return fmt.Errorf("unsupported route line")
		}
		next := driver.NextHop{Evidence: ref(i)}
		if iface, ok := strings.CutPrefix(s, "receive via "); ok {
			if r.Protocol != "connected" || !driver.InterfaceName(iface) {
				return fmt.Errorf("invalid receive path")
			}
			next.Interface = interfaceName(iface)
		} else {
			a, e := netip.ParseAddr(s)
			if e != nil || !a.Is6() || a.Zone() != "" || r.Protocol != "static" {
				return fmt.Errorf("unsupported route path")
			}
			next.Address = a.String()
		}
		r.NextHops = append(r.NextHops, next)
	}
	if noRoute {
		if r.Prefix != "" || r.Protocol != "" || paths {
			return fmt.Errorf("conflicting missing and present route")
		}
		f.Notes = append(f.Notes, "No matching IPv4 route was returned.")
		return nil
	}
	if r.Prefix == "" || !paths || len(r.NextHops) == 0 {
		return fmt.Errorf("incomplete route entry")
	}
	f.Routes = append(f.Routes, r)
	f.Notes = append(f.Notes, "Captured IOS-XE static/receive paths are RIB observations. Explicit selection, hardware installation and recursive egress remain unknown; no interface is inferred for a gateway-only path.")
	return nil
}
