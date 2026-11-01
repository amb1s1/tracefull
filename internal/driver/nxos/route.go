package nxos

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/amb1s1/tracefull/internal/driver"
)

func routes(n node, f *driver.Facts) error {
	vrfs, err := children(n, "TABLE_vrf", "ROW_vrf", false)
	if err != nil {
		return err
	}
	for _, vrf := range vrfs {
		var v struct {
			Context string `json:"vrf-name-out"`
		}
		if err := json.Unmarshal(vrf.data, &v); err != nil {
			return err
		}
		if v.Context == "" {
			return fmt.Errorf("missing route VRF")
		}
		families, e := children(vrf, "TABLE_addrf", "ROW_addrf", false)
		if e != nil {
			return e
		}
		for _, family := range families {
			var af struct {
				Family string `json:"addrf"`
			}
			if err := json.Unmarshal(family.data, &af); err != nil {
				return err
			}
			if af.Family != "ipv4" && af.Family != "ipv6" {
				return fmt.Errorf("unsupported route family")
			}
			prefixes, e := children(family, "TABLE_prefix", "ROW_prefix", true)
			if e != nil {
				return e
			}
			for _, prefix := range prefixes {
				r, e := route(prefix, v.Context, af.Family)
				if e != nil {
					return e
				}
				f.Routes = append(f.Routes, r)
			}
		}
	}
	f.Notes = append(f.Notes, "NX-OS ubest describes RIB selection, not hardware installation. Cross-VRF next-hop selection remains unknown in normalized facts because its context cannot be represented; see raw nexthop-vrf-name.")
	return nil
}
func route(n node, context, family string) (driver.Route, error) {
	r := driver.Route{Context: context, NextHops: []driver.NextHop{}, Evidence: n.ref}
	var v struct {
		Prefix string `json:"ipprefix"`
		IPv6   string `json:"ipv6prefix"`
	}
	if err := json.Unmarshal(n.data, &v); err != nil {
		return r, err
	}
	p, e := netip.ParsePrefix(v.Prefix)
	if e != nil || p.Addr().Zone() != "" || (family == "ipv4") != p.Addr().Is4() {
		return r, fmt.Errorf("invalid route prefix")
	}
	if v.IPv6 != "" {
		p6, e := netip.ParsePrefix(v.IPv6)
		if e != nil || p6 != p {
			return r, fmt.Errorf("conflicting route prefixes")
		}
	}
	r.Prefix = p.String()
	paths, e := children(n, "TABLE_path", "ROW_path", false)
	if e != nil {
		return r, e
	}
	if len(paths) == 0 {
		return r, fmt.Errorf("route without paths")
	}
	allKnown, selected := true, false
	for _, path := range paths {
		var v struct {
			IP        string `json:"ipnexthop"`
			IPv6      string `json:"ipv6nexthop"`
			Interface string `json:"ifname"`
			Protocol  string `json:"clientname"`
			Best      string `json:"ubest"`
			Context   string `json:"nexthop-vrf-name"`
		}
		if err := json.Unmarshal(path.data, &v); err != nil {
			return r, err
		}
		if v.IP != "" {
			a, e := netip.ParseAddr(v.IP)
			if e != nil || a.Zone() != "" {
				return r, fmt.Errorf("invalid next hop")
			}
			v.IP = a.String()
			if v.IPv6 != "" {
				b, e := netip.ParseAddr(v.IPv6)
				if e != nil || a != b {
					return r, fmt.Errorf("conflicting next hops")
				}
			}
		}
		if v.Interface != "" && !driver.InterfaceName(v.Interface) {
			return r, fmt.Errorf("invalid next-hop interface")
		}
		if v.IP == "" && v.Interface == "" {
			return r, fmt.Errorf("unidentified next hop")
		}
		if r.Protocol != "" && v.Protocol != r.Protocol {
			return r, fmt.Errorf("mixed route protocols")
		}
		r.Protocol = v.Protocol
		best, e := boolValue(v.Best)
		if e != nil {
			return r, e
		}
		if best == nil {
			allKnown = false
		} else if *best {
			selected = true
		}
		if v.Context != "" && v.Context != context {
			best = nil
		}
		r.NextHops = append(r.NextHops, driver.NextHop{Address: v.IP, Interface: v.Interface, Active: best, Evidence: path.ref})
	}
	if selected || allKnown {
		r.Selected = &selected
	}
	return r, nil
}
