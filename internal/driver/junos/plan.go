// Package junos interprets capture-backed Junos 24.4 observations.
package junos

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "junos" }
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, interfaces, err := driver.Selection(groups, p)
	if err != nil {
		return nil, err
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses", "ingress", "show interfaces terse | display json"))
	}
	if wanted["route"] {
		a, e := driver.Destination(p)
		if e != nil {
			return nil, e
		}
		table := "inet.0"
		if a.Is6() {
			table = "inet6.0"
		}
		out = append(out, driver.Planned("route", "route", "show route table "+table+" "+a.String()+" detail | display json"))
	}
	if wanted["version"] {
		out = append(out, driver.Planned("version", "version", "show version | display json"), driver.Planned("uptime", "version", "show system uptime | display json"))
	}
	if wanted["cpu"] {
		out = append(out, driver.Planned("cpu", "cpu", "show chassis routing-engine | display json"))
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
			out = append(out, driver.Planned("lldp", g, "show lldp neighbors | display json"))
			continue
		}
		seen := map[string]bool{}
		for _, iface := range interfaces {
			cmd := "show interfaces " + iface + " extensive | display json"
			if g == "optics" {
				iface = strings.Split(iface, ".")[0]
				if !strings.HasPrefix(iface, "et-") && !strings.HasPrefix(iface, "xe-") && !strings.HasPrefix(iface, "ge-") {
					out = append(out, driver.Command{Name: "optics-" + iface, Group: g, Status: "unsupported", Reason: "DOM is only modelled on physical Ethernet ports"})
					continue
				}
				cmd = "show interfaces diagnostics optics " + iface + " | display json"
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
		return fmt.Errorf("unsupported Junos routing context")
	}
	switch cmd {
	case "show interfaces terse | display json", "show version | display json", "show system uptime | display json", "show chassis routing-engine | display json", "show lldp neighbors | display json":
		return nil
	}
	for _, pattern := range [][2]string{{"show interfaces ", " extensive | display json"}, {"show interfaces diagnostics optics ", " | display json"}} {
		if v, ok := strings.CutPrefix(cmd, pattern[0]); ok {
			if v, ok = strings.CutSuffix(v, pattern[1]); ok && driver.InterfaceName(v) {
				return nil
			}
		}
	}
	for _, table := range []string{"inet.0", "inet6.0"} {
		if v, ok := strings.CutPrefix(cmd, "show route table "+table+" "); ok {
			if v, ok = strings.CutSuffix(v, " detail | display json"); ok {
				a, e := netip.ParseAddr(v)
				if e == nil && a.Zone() == "" && ((table == "inet.0" && a.Is4()) || (table == "inet6.0" && a.Is6())) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("command is outside the Junos read-only policy")
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
