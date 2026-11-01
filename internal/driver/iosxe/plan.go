// Package iosxe interprets capture-backed Cisco IOS-XE 17.12 observations.
package iosxe

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "cisco-ios-xe" }
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	wanted, ifaces, e := driver.Selection(groups, p)
	if e != nil {
		return nil, e
	}
	out := []driver.Command{}
	if wanted["ingress"] {
		out = append(out, driver.Planned("addresses4", "ingress", "show ip interface brief"), driver.Planned("addresses6", "ingress", "show ipv6 interface brief"))
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
		out = append(out, driver.Planned("route", "route", "show "+family+" route "+a.String()))
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
		if len(ifaces) == 0 {
			out = append(out, driver.Unavailable(g, true))
			continue
		}
		if g == "lldp" {
			out = append(out, driver.Planned(g, g, "show lldp neighbors detail"))
			continue
		}
		for _, iface := range ifaces {
			cmd := "show interfaces " + iface
			if g == "optics" {
				if !strings.HasPrefix(iface, "TenGigabitEthernet") {
					out = append(out, driver.Command{Name: g + "-" + iface, Group: g, Status: "unsupported", Reason: "only captured TenGigabitEthernet DOM layout is validated"})
					continue
				}
				cmd += " transceiver detail"
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
func (Driver) Validate(cmd string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" {
		return fmt.Errorf("unsupported IOS-XE routing context")
	}
	switch cmd {
	case "show version", "show processes cpu", "show ip interface brief", "show ipv6 interface brief", "show lldp neighbors detail":
		return nil
	}
	if s, ok := strings.CutPrefix(cmd, "show interfaces "); ok {
		if iface, optics := strings.CutSuffix(s, " transceiver detail"); optics {
			if strings.HasPrefix(iface, "TenGigabitEthernet") && driver.InterfaceName(iface) {
				return nil
			}
		} else if driver.InterfaceName(s) {
			return nil
		}
	}
	for _, family := range []string{"ip", "ipv6"} {
		if s, ok := strings.CutPrefix(cmd, "show "+family+" route "); ok {
			a, e := netip.ParseAddr(s)
			if e == nil && a.Zone() == "" && ((family == "ip" && a.Is4()) || (family == "ipv6" && a.Is6())) {
				return nil
			}
		}
	}
	return fmt.Errorf("command is outside the IOS-XE read-only policy")
}
func interfaceName(s string) string {
	for _, pair := range [][2]string{{"Te", "TenGigabitEthernet"}, {"Gi", "GigabitEthernet"}, {"Po", "Port-channel"}} {
		if strings.HasPrefix(s, pair[0]) && !strings.HasPrefix(s, pair[1]) {
			return pair[1] + strings.TrimPrefix(s, pair[0])
		}
	}
	return s
}
