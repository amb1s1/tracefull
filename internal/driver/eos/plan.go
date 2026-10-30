// Package eos interprets capture-backed Arista EOS 4.33 observations.
package eos

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "arista-eos" }
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, interfaces, err := driver.Selection(groups, p)
	if err != nil {
		return nil, err
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses4", "ingress", "show ip interface | json"), driver.Planned("addresses6", "ingress", "show ipv6 interface | json"))
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
		out = append(out, driver.Planned("route", "route", "show "+family+" route "+a.String()+" | json"))
	}
	if wanted["version"] {
		out = append(out, driver.Planned("version", "version", "show version | json"))
	}
	for _, g := range []string{"counters", "optics", "lldp", "cpu", "reverse"} {
		if !wanted[g] {
			continue
		}
		if g == "cpu" || g == "reverse" {
			out = append(out, driver.Unavailable(g, false))
			continue
		}
		if len(interfaces) == 0 {
			out = append(out, driver.Unavailable(g, true))
			continue
		}
		if g == "lldp" {
			out = append(out, driver.Planned("lldp", g, "show lldp neighbors detail | json"))
			continue
		}
		for _, iface := range interfaces {
			cmd := "show interfaces " + iface
			if g == "optics" {
				if !strings.HasPrefix(iface, "Ethernet") {
					out = append(out, driver.Command{Name: "optics-" + iface, Group: g, Status: "unsupported", Reason: "DOM is only modelled on Ethernet interfaces"})
					continue
				}
				cmd += " transceiver"
			}
			out = append(out, driver.Planned(g+"-"+iface, g, cmd+" | json"))
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
		return fmt.Errorf("unsupported EOS routing context")
	}
	switch cmd {
	case "show version | json", "show ip interface | json", "show ipv6 interface | json", "show lldp neighbors detail | json":
		return nil
	}
	if v, ok := strings.CutPrefix(cmd, "show interfaces "); ok {
		if v, ok = strings.CutSuffix(v, " | json"); ok {
			iface, optics := strings.CutSuffix(v, " transceiver")
			if driver.InterfaceName(iface) && (!optics || strings.HasPrefix(iface, "Ethernet")) {
				return nil
			}
		}
	}
	for _, family := range []string{"ip", "ipv6"} {
		if v, ok := strings.CutPrefix(cmd, "show "+family+" route "); ok {
			if v, ok = strings.CutSuffix(v, " | json"); ok {
				a, e := netip.ParseAddr(v)
				if e == nil && a.Zone() == "" && ((family == "ip" && a.Is4()) || (family == "ipv6" && a.Is6())) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("command is outside the EOS read-only policy")
}
