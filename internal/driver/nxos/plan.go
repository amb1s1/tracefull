// Package nxos interprets capture-backed Cisco NX-OS 10.5 observations.
package nxos

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "cisco-nxos" }
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, interfaces, err := driver.Selection(groups, p)
	if err != nil {
		return nil, err
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses4", "ingress", "show ip interface brief vrf all | json"), driver.Planned("addresses6", "ingress", "show ipv6 interface brief vrf all | json"))
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
	if wanted["cpu"] {
		out = append(out, driver.Planned("cpu", "cpu", "show system resources | json"))
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
			out = append(out, driver.Planned(g, g, "show lldp neighbors detail | json"))
			continue
		}
		for _, iface := range interfaces {
			cmd := "show interface " + iface
			if g == "optics" {
				if !strings.HasPrefix(iface, "Ethernet") {
					out = append(out, driver.Command{Name: g + "-" + iface, Group: g, Status: "unsupported", Reason: "only Ethernet transceiver captures validated"})
					continue
				}
				cmd += " transceiver details"
			}
			out = append(out, driver.Planned(g+"-"+iface, g, cmd+" | json"))
		}
	}
	for _, c := range out {
		if c.Status == "planned" {
			if err := d.Validate(c.Text, p); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
func (Driver) Validate(cmd string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" {
		return fmt.Errorf("unsupported NX-OS routing context")
	}
	switch cmd {
	case "show version | json", "show ip interface brief vrf all | json", "show ipv6 interface brief vrf all | json", "show system resources | json", "show lldp neighbors detail | json":
		return nil
	}
	body, ok := strings.CutSuffix(cmd, " | json")
	if !ok {
		return fmt.Errorf("NX-OS requires exact JSON display suffix")
	}
	if v, ok := strings.CutPrefix(body, "show interface "); ok {
		if iface, optics := strings.CutSuffix(v, " transceiver details"); optics {
			if strings.HasPrefix(iface, "Ethernet") && driver.InterfaceName(iface) {
				return nil
			}
		} else if driver.InterfaceName(v) {
			return nil
		}
	}
	for _, family := range []string{"ip", "ipv6"} {
		if v, ok := strings.CutPrefix(body, "show "+family+" route "); ok {
			a, e := netip.ParseAddr(v)
			if e == nil && a.Zone() == "" && ((family == "ip" && a.Is4()) || (family == "ipv6" && a.Is6())) {
				return nil
			}
		}
	}
	return fmt.Errorf("command is outside the NX-OS read-only policy")
}
