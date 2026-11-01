package routeros

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func routes(destination string, b []byte, f *driver.Facts) error {
	dest, e := netip.ParseAddr(destination)
	if e != nil || dest.Zone() != "" {
		return fmt.Errorf("invalid route observation destination")
	}
	type entry struct {
		Prefix                        string `json:"dst-address"`
		Context                       string `json:"routing-table"`
		Gateway                       string
		Immediate                     []string `json:"immediate-gw"`
		Active, Inactive, Static, BGP *bool
	}
	all, e := rows[entry](b)
	if e != nil {
		return e
	}
	longest := -1
	for i, v := range all {
		p, e := netip.ParsePrefix(v.Prefix)
		if e != nil || !p.Contains(dest) || v.Context != "main" {
			return fmt.Errorf("route outside requested destination/default context")
		}
		if v.Active != nil && *v.Active && v.Inactive != nil && *v.Inactive {
			return fmt.Errorf("conflicting route state")
		}
		proto := ""
		if v.Static != nil && *v.Static {
			proto = "static"
		}
		if v.BGP != nil && *v.BGP {
			if proto != "" {
				return fmt.Errorf("conflicting route protocol")
			}
			proto = "bgp"
		}
		if proto == "" {
			return fmt.Errorf("only captured static/BGP layouts validated")
		}
		r := driver.Route{Prefix: p.String(), Protocol: proto, Context: "default", Selected: v.Active, NextHops: []driver.NextHop{}, Evidence: fmt.Sprintf("json:/%d", i)}
		for j, gw := range v.Immediate {
			address, iface, ok := strings.Cut(gw, "%")
			if !ok || !interfaceRE.MatchString(iface) {
				return fmt.Errorf("unvalidated immediate-gateway layout")
			}
			a, e := netip.ParseAddr(address)
			if e != nil || a.Zone() != "" {
				return fmt.Errorf("invalid immediate gateway")
			}
			r.NextHops = append(r.NextHops, driver.NextHop{Address: a.String(), Interface: iface, Active: v.Active, Evidence: fmt.Sprintf("json:/%d/immediate-gw/%d", i, j)})
		}
		if len(r.NextHops) == 0 {
			if v.Gateway == "" {
				return fmt.Errorf("missing gateway")
			}
			a, e := netip.ParseAddr(v.Gateway)
			if e != nil || a.Zone() != "" {
				return fmt.Errorf("unvalidated recursive gateway")
			}
			r.NextHops = append(r.NextHops, driver.NextHop{Address: a.String(), Active: v.Active, Evidence: r.Evidence + "/gateway"})
		}
		if p.Bits() > longest {
			longest = p.Bits()
		}
		f.Routes = append(f.Routes, r)
	}
	// The device filter returns every covering prefix. Do not use a less-specific
	// default for interface discovery when a more-specific observation is present.
	f.Routes = slices.DeleteFunc(f.Routes, func(r driver.Route) bool { p, _ := netip.ParsePrefix(r.Prefix); return p.Bits() != longest })
	f.Notes = append(f.Notes, "Only the most-specific matching prefix is normalized. Missing active flags stay unknown; there is no fallback to less-specific routes. RIB activity does not establish hardware installation.")
	return nil
}
