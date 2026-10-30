package collect

import (
	"fmt"
	"net/netip"
	"slices"

	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/trace"
)

// Compare each recorded flow separately. Global next-hop addresses are comparable
// to responding addresses; unscoped link-local routes, unknown FIB state and a
// mismatched address master are not enough to assert a path discrepancy.
func comparisons(p Prepared, devices []DeviceResult) []Finding {
	out := []Finding{}
	paths := []trace.Path{{Hops: p.Trace.Hops}}
	if len(p.Trace.Paths) > 0 {
		paths = p.Trace.Paths
	}
	for _, path := range paths {
		var pmtu *int
		mtuRefs := []string{}
		for i, h := range path.Hops {
			if h.PMTU != nil && (pmtu == nil || *h.PMTU < *pmtu) {
				pmtu = h.PMTU
				mtuRefs = traceRefs(h.Evidence)
			}
			for _, d := range devices {
				if !ownedInContext(h, d, p.Plan.Context) {
					continue
				}
				routes := []driver.Route{}
				routeFiles := []string{}
				for _, c := range d.Commands {
					if c.Facts == nil || c.Stdout == nil {
						continue
					}
					for _, r := range c.Facts.Routes {
						if comparableRoute(r, p.Plan.Destination, p.Plan.Context) {
							routes = append(routes, r)
							routeFiles = append(routeFiles, c.Stdout.Path)
						}
					}
				}
				for ri, r := range routes {
					egress := []string{}
					nextIPs := []string{}
					allGlobal := len(r.NextHops) > 0
					for _, n := range r.NextHops {
						if n.Active == nil || n.Installed == nil {
							allGlobal = false
							continue
						}
						if !*n.Active || !*n.Installed {
							continue
						}
						egress = trace.AddUnique(egress, n.Interface)
						a, e := netip.ParseAddr(n.Address)
						if e != nil || a.IsLinkLocalUnicast() || a.Zone() != "" {
							allGlobal = false
							continue
						}
						nextIPs = trace.AddUnique(nextIPs, a.Unmap().String())
					}
					if len(routes) == 1 && i+1 < len(path.Hops) && path.Hops[i+1].TTL == h.TTL+1 && len(path.Hops[i+1].Addresses) > 0 && allGlobal && len(nextIPs) > 0 {
						next := path.Hops[i+1]
						overlap := false
						for _, ip := range next.Addresses {
							overlap = overlap || slices.Contains(nextIPs, ip) || sameObservedOwner(ip, nextIPs, devices)
						}
						if !overlap {
							refs := append(traceRefs(h.Evidence), traceRefs(next.Evidence)...)
							refs = append(refs, routeFiles[ri]+"#"+r.Evidence)
							f := finding("path-mismatch", d.Name, fmt.Sprintf("At TTL %d (flow %q), the later route next-hop set does not include the next recorded responder or a known address of its observed owner. Path change, another ECMP member, reply-source selection or incomplete identity may explain the difference.", h.TTL, path.ID), refs)
							f.Severity = "warn"
							out = append(out, f)
						}
					}
					if pmtu == nil {
						continue
					}
					for _, c := range d.Commands {
						if c.Facts == nil || c.Stdout == nil {
							continue
						}
						for _, counter := range c.Facts.Counters {
							if !slices.Contains(egress, counter.Interface) || counter.MTUBytes == nil || uint64(*pmtu) <= *counter.MTUBytes {
								continue
							}
							refs := append(slices.Clone(mtuRefs), routeFiles[ri]+"#"+r.Evidence, c.Stdout.Path+"#"+counter.Evidence)
							f := finding("mtu-drop", d.Name, fmt.Sprintf("Egress %s reports MTU %d bytes, below the earlier flow %q path observation of %d bytes. Times and paths may differ; this comparison does not establish a current PMTU fault.", counter.Interface, *counter.MTUBytes, path.ID, *pmtu), refs)
							f.Severity = "warn"
							out = append(out, f)
						}
					}
				}
			}
		}
	}
	return out
}
func traceRefs(refs []string) []string {
	out := []string{}
	for _, r := range refs {
		out = append(out, "trace.input#"+r)
	}
	return out
}
func comparableRoute(r driver.Route, destination, context string) bool {
	prefix, e := netip.ParsePrefix(r.Prefix)
	addr, e2 := netip.ParseAddr(destination)
	return e == nil && e2 == nil && prefix.Contains(addr) && r.Context == context && r.Selected != nil && *r.Selected && r.Installed != nil && *r.Installed
}
func ownedInContext(h trace.Hop, d DeviceResult, context string) bool {
	for ip, m := range h.Matches {
		if m.Device != d.Name {
			continue
		}
		for _, c := range d.Commands {
			if c.Facts == nil {
				continue
			}
			for _, a := range c.Facts.Addresses {
				if a.Address == ip && (a.Master == context || (context == "default" && a.Master == "")) {
					return true
				}
			}
		}
	}
	return false
}
func sameObservedOwner(ip string, nextIPs []string, devices []DeviceResult) bool {
	// Shared/anycast addresses may have several owners. Only unique overlapping
	// ownership suppresses a discrepancy, without choosing one arbitrary device.
	owners := func(address string) []string {
		out := []string{}
		for _, d := range devices {
			for _, c := range d.Commands {
				if c.Facts == nil {
					continue
				}
				for _, a := range c.Facts.Addresses {
					if a.Address == address {
						out = trace.AddUnique(out, d.Name)
					}
				}
			}
		}
		return out
	}
	known := owners(ip)
	if len(known) != 1 {
		return false
	}
	for _, n := range nextIPs {
		other := owners(n)
		if len(other) == 1 && other[0] == known[0] {
			return true
		}
	}
	return false
}
