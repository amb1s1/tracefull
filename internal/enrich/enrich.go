package enrich

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Resolver interface {
	LookupAddr(context.Context, string) ([]string, error)
	LookupTXT(context.Context, string) ([]string, error)
}
type Options struct {
	RDNS, ASN bool
	Timeout   time.Duration
	Overrides map[string]string
}

type result struct{ names, asn, notes []string }

// Apply mutates the caller-owned trace. Lookups are sequential, cached per run and bounded.
func Apply(ctx context.Context, t *trace.Trace, inv inventory.Inventory, r Resolver, o Options) error {
	if e := inv.ValidateOverrides(o.Overrides); e != nil {
		return e
	}
	if o.Timeout <= 0 {
		return fmt.Errorf("DNS timeout must be positive")
	}
	if (o.RDNS || o.ASN) && r == nil {
		return fmt.Errorf("DNS resolver unavailable")
	}
	cache := map[string]result{}
	apply := func(hops []trace.Hop) error {
		for i := range hops {
			for _, ip := range hops[i].Addresses {
				if e := ctx.Err(); e != nil {
					return e
				}
				v, ok := cache[ip]
				if !ok {
					v = lookup(ctx, ip, r, o)
					cache[ip] = v
				}
				applyResult(&hops[i], ip, v, inv, o.Overrides)
			}
		}
		return ctx.Err()
	}
	if e := apply(t.Hops); e != nil {
		return e
	}
	for i := range t.Paths {
		if e := apply(t.Paths[i].Hops); e != nil {
			return e
		}
	}
	return nil
}
func lookup(ctx context.Context, ip string, r Resolver, o Options) result {
	v := result{}
	if o.RDNS {
		c, cancel := context.WithTimeout(ctx, o.Timeout)
		names, e := r.LookupAddr(c, ip)
		cancel()
		if e != nil || len(names) == 0 {
			v.notes = append(v.notes, "rDNS unavailable for "+ip)
		} else {
			v.names = append([]string(nil), names...)
			sort.Strings(v.names)
		}
	}
	if o.ASN {
		if q := ASNQuery(ip); q != "" {
			c, cancel := context.WithTimeout(ctx, o.Timeout)
			txt, e := r.LookupTXT(c, q)
			cancel()
			if e == nil {
				v.asn = parseASN(txt)
			}
			if e != nil || len(v.asn) == 0 {
				v.notes = append(v.notes, "ASN unavailable for "+ip)
			}
		}
	}
	return v
}
func parseASN(txt []string) []string {
	out := []string{}
	for _, line := range txt {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		for _, as := range strings.Fields(parts[0]) {
			if _, e := strconv.ParseUint(as, 10, 32); e == nil {
				out = trace.AddUnique(out, as)
			}
		}
	}
	sort.Strings(out)
	return out
}
func applyResult(h *trace.Hop, ip string, v result, inv inventory.Inventory, overrides map[string]string) {
	if len(v.names) > 0 {
		h.RDNS[ip] = append([]string(nil), v.names...)
	}
	if len(v.asn) > 0 {
		h.ASN[ip] = append([]string(nil), v.asn...)
	}
	h.Notes = append(h.Notes, v.notes...)
	a, _ := netip.ParseAddr(ip)
	if a.IsPrivate() {
		h.Tags = trace.AddUnique(h.Tags, "private")
	}
	if a.IsLoopback() {
		h.Tags = trace.AddUnique(h.Tags, "loopback")
	}
	if a.IsLinkLocalUnicast() {
		h.Tags = trace.AddUnique(h.Tags, "link-local")
	}
	names := append([]string(nil), v.names...)
	if len(h.Addresses) == 1 {
		names = append(names, h.Names...)
	}
	h.Matches[ip] = inv.Match(ip, names, overrides)
}

// ASNQuery excludes non-public and documentation ranges before constructing Cymru DNS.
func ASNQuery(ip string) string {
	a, e := netip.ParseAddr(ip)
	if e != nil {
		return ""
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() {
		return ""
	}
	for _, s := range []string{"192.0.0.0/24", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "198.18.0.0/15", "100.64.0.0/10", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return ""
		}
	}
	if a.Is4() {
		b := a.As4()
		return fmt.Sprintf("%d.%d.%d.%d.origin.asn.cymru.com", b[3], b[2], b[1], b[0])
	}
	b := a.As16()
	hex := fmt.Sprintf("%032x", b)
	parts := []string{}
	for i := len(hex) - 1; i >= 0; i-- {
		parts = append(parts, string(hex[i]))
	}
	return strings.Join(parts, ".") + ".origin6.asn.cymru.com"
}
