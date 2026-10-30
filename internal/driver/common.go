package driver

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

var interfaceToken = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:/-]{0,62}$`)

func InterfaceName(s string) bool { return interfaceToken.MatchString(s) }
func Selection(groups []string, p Params) (map[string]bool, []string, error) {
	if p.VRF != "" && p.VRF != "default" {
		return nil, nil, fmt.Errorf("routing context %q is not capture-validated; no fallback", p.VRF)
	}
	wanted := map[string]bool{}
	for _, g := range groups {
		if !slices.Contains([]string{"ingress", "route", "counters", "optics", "lldp", "version", "cpu", "reverse"}, g) {
			return nil, nil, fmt.Errorf("unknown collection group %q", g)
		}
		wanted[g] = true
	}
	if wanted["counters"] || wanted["optics"] || wanted["lldp"] {
		wanted["ingress"], wanted["route"] = true, true
	}
	if len(p.Interfaces) > 256 {
		return nil, nil, fmt.Errorf("discovery exceeds 256 interfaces")
	}
	interfaces := slices.Clone(p.Interfaces)
	slices.Sort(interfaces)
	interfaces = slices.Compact(interfaces)
	for _, n := range interfaces {
		if !InterfaceName(n) {
			return nil, nil, fmt.Errorf("invalid interface name %q", n)
		}
	}
	return wanted, interfaces, nil
}
func Destination(p Params) (netip.Addr, error) {
	a, e := netip.ParseAddr(p.Destination)
	if e != nil || a.Zone() != "" {
		return a, fmt.Errorf("route lookup requires an unscoped numeric destination")
	}
	return a.Unmap(), nil
}
func Planned(name, group, command string) Command {
	return Command{Name: name, Group: group, Text: command, Status: "planned"}
}
func Unavailable(group string, deferred bool) Command {
	status, reason := "unsupported", "no capture-validated implementation"
	if deferred {
		status, reason = "deferred", "requires interfaces from ingress/route discovery"
	}
	return Command{Name: group, Group: group, Status: status, Reason: reason}
}
func JSONPart(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }
