package routeros

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func (Driver) Parse(name string, b []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(b) == 0 || len(b) > 1<<20 {
		return f, fmt.Errorf("empty or oversized RouterOS output")
	}
	var err error
	switch {
	case name == "version" || name == "cpu":
		err = resource(name, b, &f)
	case strings.HasPrefix(name, "optics-"):
		err = optics(strings.TrimPrefix(name, "optics-"), b, &f)
	case name == "addresses4" || name == "addresses6":
		err = addresses(name, b, &f)
	case strings.HasPrefix(name, "route-"):
		err = routes(strings.TrimPrefix(name, "route-"), b, &f)
	case name == "lldp":
		err = neighbors(b, &f)
	case strings.HasPrefix(name, "counters-"):
		err = counters(strings.TrimPrefix(name, "counters-"), b, &f)
	default:
		err = fmt.Errorf("unknown RouterOS observation")
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}
func rows[T any](b []byte) ([]*T, error) {
	var out []*T
	if err := json.Unmarshal(b, &out); err != nil || out == nil {
		return nil, fmt.Errorf("expected JSON result array")
	}
	for _, v := range out {
		if v == nil {
			return nil, fmt.Errorf("null result row")
		}
	}
	return out, nil
}
func contextName(s string) string {
	if s == "main" {
		return "default"
	}
	return s
}
func addresses(name string, b []byte, f *driver.Facts) error {
	type entry struct {
		Address, Interface, VRF string
		Disabled, Invalid       *bool
	}
	all, e := rows[entry](b)
	if e != nil {
		return e
	}
	for i, v := range all {
		if !interfaceRE.MatchString(v.Interface) || v.VRF == "" {
			return fmt.Errorf("missing address interface/context")
		}
		p, e := netip.ParsePrefix(v.Address)
		if e != nil || (name == "addresses4") != p.Addr().Is4() {
			return fmt.Errorf("invalid address prefix")
		}
		if (v.Disabled != nil && *v.Disabled) || (v.Invalid != nil && *v.Invalid) {
			continue
		}
		length := p.Bits()
		context := contextName(v.VRF)
		f.Addresses = append(f.Addresses, driver.Address{Interface: v.Interface, Address: p.Addr().String(), PrefixLength: &length, RoutingContext: &context, Evidence: fmt.Sprintf("json:/%d", i)})
	}
	f.Notes = append(f.Notes, "RouterOS main is normalized as default; other reported VRF names are preserved.")
	return nil
}
func neighbors(b []byte, f *driver.Facts) error {
	type entry struct {
		Interfaces        []string `json:"interface"`
		Protocols         []string `json:"discovered-by"`
		Identity          string
		Chassis           string `json:"mac-address"`
		Port              string `json:"interface-name"`
		Address, Address6 string
	}
	all, e := rows[entry](b)
	if e != nil {
		return e
	}
	for i, v := range all {
		if !slices.Contains(v.Protocols, "lldp") {
			continue
		}
		if len(v.Interfaces) == 0 {
			return fmt.Errorf("LLDP neighbor without interface")
		}
		addresses := []string{}
		for _, s := range []string{v.Address, v.Address6} {
			if s == "" {
				continue
			}
			a, e := netip.ParseAddr(s)
			if e != nil || a.Zone() != "" {
				return fmt.Errorf("invalid neighbor address")
			}
			if !slices.Contains(addresses, a.String()) {
				addresses = append(addresses, a.String())
			}
		}
		for _, iface := range v.Interfaces {
			if !interfaceRE.MatchString(iface) {
				return fmt.Errorf("invalid neighbor interface")
			}
			f.Neighbors = append(f.Neighbors, driver.Neighbor{Interface: iface, Name: v.Identity, ChassisID: v.Chassis, PortID: v.Port, ManagementAddresses: slices.Clone(addresses), Evidence: fmt.Sprintf("json:/%d", i)})
		}
	}
	f.Notes = append(f.Notes, "RouterOS neighbor interface lists can include bridge membership. Each reported interface is retained; no physical topology is inferred.")
	return nil
}
func counters(iface string, b []byte, f *driver.Facts) error {
	type entry struct {
		ID        string `json:".id"`
		Name      string
		Running   *bool
		MTU       *uint64 `json:"actual-mtu"`
		RXErrors  *uint64 `json:"rx-error"`
		TXErrors  *uint64 `json:"tx-error"`
		RXDrops   *uint64 `json:"rx-drop"`
		TXDrops   *uint64 `json:"tx-queue-drop"`
		RXPackets *uint64 `json:"rx-packet"`
	}
	all, e := rows[entry](b)
	if e != nil {
		return e
	}
	if len(all) != 2 || all[0].ID == "" || all[0].ID != all[1].ID || all[0].Name != iface || all[1].Name != iface || all[0].MTU == nil || all[1].RXPackets == nil {
		return fmt.Errorf("incomplete or mismatched interface detail/statistics pair")
	}
	a, s := all[0], all[1]
	state := ""
	if s.Running != nil {
		state = "down"
		if *s.Running {
			state = "up"
		}
	}
	f.Counters = append(f.Counters, driver.Counter{Interface: iface, State: state, MTUBytes: a.MTU, RXErrors: s.RXErrors, TXErrors: s.TXErrors, RXDrops: s.RXDrops, TXDrops: s.TXDrops, Evidence: "json:"})
	f.Notes = append(f.Notes, "Interface details and cumulative statistics are consecutive reads, not one atomic sample. TX drops are reported queue drops; missing error/drop counters, line speed and MTU framing stay unknown.")
	return nil
}
