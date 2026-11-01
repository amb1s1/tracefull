// Package routeros interprets capture-backed MikroTik RouterOS 7.24 observations.
package routeros

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "mikrotik-routeros" }

const jsonPrefix = ":put [:serialize to=json options=json.no-string-conversion value=["

var interfaceRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,62}$`)

func serialized(read string) string { return jsonPrefix + read + "]]" }
func counterCommand(iface string) string {
	return ":put [:serialize to=json options=json.no-string-conversion value=([/interface print detail as-value where name=" + iface + "],[/interface print stats-detail as-value where name=" + iface + "])]"
}
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, ifaces, e := driver.Selection(groups, p)
	if e != nil {
		return nil, e
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses4", "ingress", serialized("/ip address print as-value")), driver.Planned("addresses6", "ingress", serialized("/ipv6 address print as-value")))
	}
	if wanted["route"] {
		a, e := driver.Destination(p)
		if e != nil {
			return nil, e
		}
		family := "ip"
		if a.Is6() {
			family = "ipv6"
		}
		out = append(out, driver.Planned("route-"+a.String(), "route", serialized("/"+family+" route print detail as-value where "+a.String()+" in dst-address and routing-table=main")))
	}
	for _, g := range []string{"version", "cpu"} {
		if wanted[g] {
			out = append(out, driver.Planned(g, g, "/system resource print"))
		}
	}
	for _, g := range []string{"counters", "optics", "lldp", "reverse"} {
		if !wanted[g] {
			continue
		}
		if g == "reverse" {
			out = append(out, driver.Unavailable(g, false))
			continue
		}
		if len(ifaces) == 0 {
			out = append(out, driver.Unavailable(g, true))
			continue
		}
		if g == "lldp" {
			out = append(out, driver.Planned(g, g, serialized("/ip neighbor print detail as-value")))
			continue
		}
		for _, iface := range ifaces {
			cmd := counterCommand(iface)
			if g == "optics" {
				if !opticalInterface(iface) {
					out = append(out, driver.Command{Name: g + "-" + iface, Group: g, Status: "unsupported", Reason: "no validated Ethernet monitor mapping for this interface"})
					continue
				}
				cmd = "/interface ethernet monitor " + iface + " once"
			}
			out = append(out, driver.Planned(g+"-"+iface, g, cmd))
		}
	}
	for _, c := range out {
		if c.Status == "planned" {
			if e := d.Validate(c.Text, p); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
func opticalInterface(s string) bool {
	return interfaceRE.MatchString(s) && (strings.HasPrefix(s, "sfp") || strings.HasPrefix(s, "ether") || strings.HasPrefix(s, "qsfp"))
}
func (Driver) Validate(cmd string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" {
		return fmt.Errorf("unsupported RouterOS routing context")
	}
	if cmd == "/system resource print" {
		return nil
	}
	for _, read := range []string{"/ip address print as-value", "/ipv6 address print as-value", "/ip neighbor print detail as-value"} {
		if cmd == serialized(read) {
			return nil
		}
	}
	if s, ok := strings.CutPrefix(cmd, "/interface ethernet monitor "); ok {
		if iface, ok := strings.CutSuffix(s, " once"); ok && opticalInterface(iface) {
			return nil
		}
	}
	// Compare the entire compound read, including the repeated interface selector.
	const cp = ":put [:serialize to=json options=json.no-string-conversion value=([/interface print detail as-value where name="
	if s, ok := strings.CutPrefix(cmd, cp); ok {
		iface, _, ok := strings.Cut(s, "]")
		if ok && interfaceRE.MatchString(iface) && cmd == counterCommand(iface) {
			return nil
		}
	}
	for _, family := range []string{"ip", "ipv6"} {
		prefix := jsonPrefix + "/" + family + " route print detail as-value where "
		if s, ok := strings.CutPrefix(cmd, prefix); ok {
			if ip, ok := strings.CutSuffix(s, " in dst-address and routing-table=main]]"); ok {
				a, e := netip.ParseAddr(ip)
				if e == nil && a.Zone() == "" && ((family == "ip" && a.Is4()) || (family == "ipv6" && a.Is6())) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("command is outside the RouterOS read-only policy")
}
