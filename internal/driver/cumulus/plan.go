// Package cumulus interprets capture-backed Cumulus 5.16 observations.
package cumulus

import (
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type Driver struct{}

func (Driver) Platform() string { return "nvidia-cumulus" }

var ifaceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,62}$`)

func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	if p.VRF != "" && p.VRF != "default" && p.VRF != "mgmt" {
		return nil, fmt.Errorf("cumulus routing context %q is not capture-validated; no fallback", p.VRF)
	}
	wanted := map[string]bool{}
	for _, g := range groups {
		if !slices.Contains([]string{"ingress", "route", "counters", "lldp", "version", "cpu", "optics", "reverse"}, g) {
			return nil, fmt.Errorf("unknown collection group %q", g)
		}
		wanted[g] = true
	}
	if wanted["counters"] || wanted["lldp"] || wanted["optics"] {
		wanted["ingress"], wanted["route"] = true, true
	}
	commands := []driver.Command{}
	add := func(name, group, text string) {
		commands = append(commands, driver.Command{Name: name, Group: group, Text: text, Status: "planned"})
	}
	if wanted["ingress"] {
		if p.VRF == "mgmt" {
			add("addresses-detailed", "ingress", "ip -j -d address show")
		} else {
			add("addresses", "ingress", "ip -j address show")
		}
	}
	if wanted["route"] {
		addr, err := netip.ParseAddr(p.Destination)
		if err != nil || addr.Zone() != "" {
			return nil, fmt.Errorf("route lookup requires an unscoped numeric destination")
		}
		addr = addr.Unmap()
		family := "ip"
		if addr.Is6() {
			family = "ipv6"
		}
		name := "route"
		if p.VRF == "mgmt" {
			name = "route-mgmt"
		}
		add(name, "route", routePrefix(family, p)+addr.String()+" json'")
	}
	if wanted["version"] {
		add("version", "version", "cat /etc/os-release")
		add("uptime", "version", "cat /proc/uptime")
	}
	if wanted["cpu"] {
		add("cpu", "cpu", "cat /proc/loadavg")
	}
	if len(p.Interfaces) > 256 {
		return nil, fmt.Errorf("discovery exceeds 256 interfaces per device")
	}
	interfaces := slices.Clone(p.Interfaces)
	slices.Sort(interfaces)
	interfaces = slices.Compact(interfaces)
	for _, iface := range interfaces {
		if !ifaceName.MatchString(iface) {
			return nil, fmt.Errorf("invalid discovered interface %q", iface)
		}
	}
	for _, g := range []string{"counters", "lldp", "optics", "reverse"} {
		if !wanted[g] {
			continue
		}
		if g == "reverse" {
			commands = append(commands, driver.Command{Name: g, Group: g, Status: "unsupported", Reason: "no capture-validated implementation"})
			continue
		}
		if len(interfaces) == 0 {
			commands = append(commands, driver.Command{Name: g, Group: g, Status: "deferred", Reason: "requires interfaces from ingress/route discovery"})
			continue
		}
		if g == "counters" {
			for _, iface := range interfaces {
				add("counters-"+iface, g, "ip -j -s link show dev "+iface)
			}
		}
		if g == "optics" {
			for _, iface := range interfaces {
				add("optics-"+iface, g, sudo(p)+"ethtool -m "+iface)
			}
		}
		if g == "lldp" {
			add("lldp", g, sudo(p)+"/usr/sbin/lldpctl -f json")
		}
	}
	for _, c := range commands {
		if c.Status == "planned" {
			if err := d.Validate(c.Text, p); err != nil {
				return nil, err
			}
		}
	}
	return commands, nil
}
func sudo(p driver.Params) string {
	if p.Sudo {
		return "sudo -n "
	}
	return ""
}

// Validate matches full fixed commands or validates each typed substitution.
// No arbitrary show-prefix, setup exception or pipeline is allowed.
func (Driver) Validate(command string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" && p.VRF != "mgmt" {
		return fmt.Errorf("unsupported routing context")
	}
	switch command {
	case "cat /etc/os-release", "cat /proc/uptime", "cat /proc/loadavg":
		return nil
	}
	if (p.VRF == "mgmt" && command == "ip -j -d address show") || (p.VRF != "mgmt" && command == "ip -j address show") {
		return nil
	}
	if command == sudo(p)+"/usr/sbin/lldpctl -f json" {
		return nil
	}
	if iface, ok := strings.CutPrefix(command, "ip -j -s link show dev "); ok && ifaceName.MatchString(iface) {
		return nil
	}
	if iface, ok := strings.CutPrefix(command, sudo(p)+"ethtool -m "); ok && ifaceName.MatchString(iface) {
		return nil
	}
	for _, family := range []string{"ip", "ipv6"} {
		prefix := routePrefix(family, p)
		if value, ok := strings.CutPrefix(command, prefix); ok {
			if value, ok = strings.CutSuffix(value, " json'"); ok {
				a, err := netip.ParseAddr(value)
				if err == nil && a.Zone() == "" && ((family == "ipv6" && a.Is6()) || (family == "ip" && a.Is4())) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("command is outside the Cumulus read-only policy")
}

func routePrefix(family string, p driver.Params) string {
	prefix := sudo(p) + "vtysh -c 'show " + family + " route "
	if p.VRF == "mgmt" {
		prefix += "vrf mgmt "
	}
	return prefix
}
