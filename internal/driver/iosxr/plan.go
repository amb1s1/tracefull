// Package iosxr interprets capture-backed Cisco IOS XR 25.4 observations.
package iosxr

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "cisco-ios-xr" }

var opticalPort = regexp.MustCompile(`^(?:FourHundredGigE|HundredGigE|FortyGigE|TwentyFiveGigE|TenGigE)([0-9]+/[0-9]+/[0-9]+/[0-9]+)$`)
var controllerPort = regexp.MustCompile(`^[0-9]+/[0-9]+/[0-9]+/[0-9]+$`)

func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, interfaces, err := driver.Selection(groups, p)
	if err != nil {
		return nil, err
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses4", "ingress", "show ipv4 interface brief"), driver.Planned("addresses6", "ingress", "show ipv6 interface brief"))
	}
	if wanted["route"] {
		a, e := driver.Destination(p)
		if e != nil {
			return nil, e
		}
		family := "ipv4"
		if a.Is6() {
			family = "ipv6"
		}
		out = append(out, driver.Planned("route", "route", "show route "+family+" "+a.String()+" detail"))
	}
	if wanted["version"] {
		out = append(out, driver.Planned("version", "version", "show version"))
	}
	if wanted["cpu"] {
		out = append(out, driver.Planned("cpu", "cpu", "show processes cpu"))
	}
	for _, g := range []string{"counters", "optics", "lldp", "reverse"} {
		if !wanted[g] {
			continue
		}
		if g == "reverse" {
			out = append(out, driver.Unavailable(g, false))
			continue
		}
		if len(interfaces) == 0 {
			out = append(out, driver.Unavailable(g, true))
			continue
		}
		if g == "lldp" {
			out = append(out, driver.Planned("lldp", g, "show lldp neighbors detail"))
			continue
		}
		seen := map[string]bool{}
		for _, iface := range interfaces {
			cmd := "show interfaces " + iface
			if g == "optics" {
				iface = strings.Split(iface, ".")[0]
				m := opticalPort.FindStringSubmatch(iface)
				if len(m) == 0 {
					out = append(out, driver.Command{Name: "optics-" + iface, Group: g, Status: "unsupported", Reason: "no validated optics controller mapping for this interface"})
					continue
				}
				cmd = "show controllers optics " + m[1]
			}
			if seen[cmd] {
				continue
			}
			seen[cmd] = true
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
func (Driver) Validate(cmd string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" {
		return fmt.Errorf("unsupported IOS XR routing context")
	}
	switch cmd {
	case "show version", "show ipv4 interface brief", "show ipv6 interface brief", "show lldp neighbors detail", "show processes cpu":
		return nil
	}
	if v, ok := strings.CutPrefix(cmd, "show interfaces "); ok && driver.InterfaceName(v) {
		return nil
	}
	if v, ok := strings.CutPrefix(cmd, "show controllers optics "); ok && controllerPort.MatchString(v) {
		return nil
	}
	for _, family := range []string{"ipv4", "ipv6"} {
		if v, ok := strings.CutPrefix(cmd, "show route "+family+" "); ok {
			if v, ok = strings.CutSuffix(v, " detail"); ok {
				a, e := netip.ParseAddr(v)
				if e == nil && a.Zone() == "" && ((family == "ipv4" && a.Is4()) || (family == "ipv6" && a.Is6())) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("command is outside the IOS XR read-only policy")
}

// ExpandInterfaces keeps logical ownership and observes the physical parent too.
// A subinterface's counters do not replace its parent port's counters/LLDP/DOM.
func (Driver) ExpandInterfaces(input []string) []string {
	out := slices.Clone(input)
	for _, iface := range input {
		if parent, _, ok := strings.Cut(iface, "."); ok {
			out = append(out, parent)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
