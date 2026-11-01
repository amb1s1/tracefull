package iosxe

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

var versionRE = regexp.MustCompile(`(?m)^Cisco IOS XE Software, Version ([0-9.]+)\s*$`)
var ipv6Row = regexp.MustCompile(`^(\S+)\s+\[([^/]+)/([^]]+)\]$`)
var cpuRE = regexp.MustCompile(`CPU utilization for five seconds: ([0-9]+)%/([0-9]+)%; one minute: ([0-9]+)%; five minutes: ([0-9]+)%`)

func ref(i int) string { return fmt.Sprintf("line:%d", i+1) }
func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized IOS-XE output")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "%") && l != "% Network not in table" {
			return f, fmt.Errorf("device reported: %s", l)
		}
	}
	var e error
	switch {
	case name == "version":
		m := versionRE.FindStringSubmatch(text)
		if m == nil {
			e = fmt.Errorf("missing IOS-XE version")
		} else {
			f.Version = &driver.Version{OS: "Cisco IOS-XE", Version: m[1], Evidence: "file"}
			f.Notes = append(f.Notes, "Reported uptime includes rounded year/week units; normalized uptime remains unknown.")
		}
	case name == "addresses4":
		e = addresses4(lines, &f)
	case name == "addresses6":
		e = addresses6(lines, &f)
	case name == "cpu":
		e = cpu(text, &f)
	case name == "route":
		e = route(lines, &f)
	case name == "lldp":
		e = lldp(lines, &f)
	case strings.HasPrefix(name, "counters-"):
		e = counters(lines, &f)
	case strings.HasPrefix(name, "optics-"):
		e = optics(strings.TrimPrefix(name, "optics-"), lines, &f)
	default:
		e = fmt.Errorf("unknown IOS-XE observation")
	}
	if e != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, e)
	}
	return f, nil
}
func addresses4(lines []string, f *driver.Facts) error {
	header := false
	for i, l := range lines {
		if strings.Contains(l, "IP-Address") && strings.Contains(l, "Protocol") {
			header = true
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !header {
			return fmt.Errorf("missing IPv4 interface header")
		}
		fields := strings.Fields(l)
		if len(fields) < 6 || !driver.InterfaceName(fields[0]) {
			return fmt.Errorf("invalid IPv4 row")
		}
		if fields[1] == "unassigned" {
			continue
		}
		a, e := netip.ParseAddr(fields[1])
		if e != nil || !a.Is4() {
			return fmt.Errorf("invalid IPv4 address")
		}
		f.Addresses = append(f.Addresses, driver.Address{Interface: interfaceName(fields[0]), Address: a.String(), Evidence: ref(i)})
	}
	if !header {
		return fmt.Errorf("missing IPv4 interface table")
	}
	f.Notes = append(f.Notes, "Brief address output omits prefix lengths and routing-context membership; neither is inferred.")
	return nil
}
func addresses6(lines []string, f *driver.Facts) error {
	iface := ""
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if m := ipv6Row.FindStringSubmatch(l); m != nil {
			if !driver.InterfaceName(m[1]) {
				return fmt.Errorf("invalid IPv6 interface")
			}
			iface = interfaceName(m[1])
			continue
		}
		if iface == "" {
			return fmt.Errorf("missing IPv6 interface row")
		}
		if l == "unassigned" {
			continue
		}
		a, e := netip.ParseAddr(l)
		if e != nil || !a.Is6() || a.Zone() != "" {
			return fmt.Errorf("invalid IPv6 address")
		}
		f.Addresses = append(f.Addresses, driver.Address{Interface: iface, Address: a.String(), Evidence: ref(i)})
	}
	if iface == "" {
		return fmt.Errorf("missing IPv6 interface rows")
	}
	f.Notes = append(f.Notes, "Brief IPv6 output omits prefix lengths and routing-context membership; these remain unknown.")
	return nil
}
func cpu(text string, f *driver.Facts) error {
	all := cpuRE.FindAllStringSubmatch(text, -1)
	if len(all) != 1 {
		return fmt.Errorf("expected one CPU summary")
	}
	v := make([]float64, 4)
	for i := range v {
		x, e := strconv.ParseFloat(all[0][i+1], 64)
		if e != nil || x < 0 || x > 100 {
			return fmt.Errorf("invalid CPU percentage")
		}
		v[i] = x
	}
	if v[1] > v[0] {
		return fmt.Errorf("interrupt utilization exceeds total")
	}
	f.CPU = &driver.CPU{Utilization: &v[0], Utilization1: &v[2], Utilization5: &v[3], Evidence: "file"}
	f.Notes = append(f.Notes, "IOS-XE CPU utilization is the reported five-second total (including interrupts); one/five-minute percentages are separate. No load average is inferred.")
	return nil
}
func lldp(lines []string, f *driver.Facts) error {
	var current *driver.Neighbor
	total := -1
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if total >= 0 && l != "" {
			return fmt.Errorf("content after LLDP summary")
		}
		if count, ok := strings.CutPrefix(l, "Total entries displayed:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(count))
			if err != nil || n < 0 {
				return fmt.Errorf("invalid LLDP count")
			}
			total = n
			continue
		}
		if s, ok := strings.CutPrefix(l, "Local Intf: "); ok {
			if !driver.InterfaceName(s) {
				return fmt.Errorf("invalid LLDP local interface")
			}
			f.Neighbors = append(f.Neighbors, driver.Neighbor{Interface: interfaceName(s), ManagementAddresses: []string{}, Evidence: ref(i)})
			current = &f.Neighbors[len(f.Neighbors)-1]
			continue
		}
		if current == nil {
			continue
		}
		for prefix, p := range map[string]*string{"Chassis id: ": &current.ChassisID, "Port id: ": &current.PortID, "System Name: ": &current.Name} {
			if s, ok := strings.CutPrefix(l, prefix); ok {
				*p = s
			}
		}
		for _, prefix := range []string{"IP: ", "IPV6: "} {
			if s, ok := strings.CutPrefix(l, prefix); ok {
				a, e := netip.ParseAddr(s)
				if e != nil || a.Zone() != "" {
					return fmt.Errorf("invalid LLDP management address")
				}
				current.ManagementAddresses = append(current.ManagementAddresses, a.String())
			}
		}
	}
	if total != len(f.Neighbors) {
		return fmt.Errorf("incomplete LLDP observation")
	}
	for _, n := range f.Neighbors {
		if n.ChassisID == "" || n.PortID == "" {
			return fmt.Errorf("incomplete LLDP neighbor")
		}
	}
	return nil
}
