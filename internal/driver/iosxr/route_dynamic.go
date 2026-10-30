package iosxr

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

var (
	isisProtocol = regexp.MustCompile(`^isis [A-Za-z0-9_-]+$`)
	isisHop      = regexp.MustCompile(`^(\S+), from (\S+), via (\S+)$`)
	isisMetric   = regexp.MustCompile(`^Route metric is [0-9]+$`)
	isisFooter   = regexp.MustCompile(`^Route version is 0x[0-9a-fA-F]+ \([0-9]+\)$`)
	isisMetadata = regexp.MustCompile(`^(Label: (None|0x[0-9a-fA-F]+ \([0-9]+\))|Tunnel ID: None|Binding Label: None|Extended communities count: 0|Path id:[0-9]+\s+Path ref count:[0-9]+|NHID: 0x[0-9a-fA-F]+ \(Ref: [0-9]+\))$`)
)

// Only captured primary IS-IS descriptor blocks are accepted. Backup/recursive
// layouts must not silently become active interfaces for downstream collection.
func isisNextHops(lines []string, r driver.Route) ([]driver.NextHop, error) {
	hops := []driver.NextHop{}
	opened, closed, metric := false, false, false
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "Routing Descriptor Blocks" {
			if opened {
				return nil, fmt.Errorf("duplicate route descriptors")
			}
			opened = true
			continue
		}
		if !opened || closed || line == "" {
			continue
		}
		if isisFooter.MatchString(line) && strings.HasPrefix(raw, "  Route version") {
			closed = true
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 4 {
			if len(hops) > 0 && !metric {
				return nil, fmt.Errorf("missing IS-IS path metric")
			}
			n, err := isisNextHop(line, r)
			if err != nil {
				return nil, err
			}
			n.Evidence = ref(i)
			hops = append(hops, n)
			metric = false
		} else if indent == 6 && len(hops) > 0 {
			if isisMetric.MatchString(line) {
				if metric {
					return nil, fmt.Errorf("duplicate IS-IS path metric")
				}
				metric = true
			} else if !isisMetadata.MatchString(line) {
				return nil, fmt.Errorf("unvalidated IS-IS path attribute")
			}
		} else {
			return nil, fmt.Errorf("unvalidated IS-IS descriptor layout")
		}
	}
	if !closed || len(hops) == 0 || !metric {
		return nil, fmt.Errorf("incomplete IS-IS descriptors")
	}
	return hops, nil
}

func isisNextHop(line string, r driver.Route) (driver.NextHop, error) {
	m := isisHop.FindStringSubmatch(line)
	if m == nil || !driver.InterfaceName(m[3]) {
		return driver.NextHop{}, fmt.Errorf("invalid IS-IS next hop")
	}
	prefix, err := netip.ParsePrefix(r.Prefix)
	if err != nil {
		return driver.NextHop{}, err
	}
	for _, text := range m[1:3] {
		a, e := netip.ParseAddr(text)
		if e != nil || a.Zone() != "" || a.Is4In6() || a.IsUnspecified() || a.IsMulticast() || a.Is4() != prefix.Addr().Is4() {
			return driver.NextHop{}, fmt.Errorf("invalid IS-IS next-hop or originator address")
		}
	}
	// 'from' is the originator, not another next hop. Installed refers to the RIB;
	// hardware installation remains unknown, including for labeled routes.
	return driver.NextHop{Address: m[1], Interface: m[3], Active: r.Selected}, nil
}
