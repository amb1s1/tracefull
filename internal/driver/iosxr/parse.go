package iosxr

import (
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

var (
	speedRE     = regexp.MustCompile(`(?:Full|Half)-duplex, ([0-9]+)Mb/s,`)
	v6row       = regexp.MustCompile(`^(\S+)\s+\[([^/]+)/([^]]+)\]$`)
	versionRE   = regexp.MustCompile(`Cisco IOS XR Software, Version ([\w.-]+)`)
	uptimeRE    = regexp.MustCompile(`uptime is ([0-9]+) days?, ([0-9]+) hours?, ([0-9]+) minutes?`)
	interfaceRE = regexp.MustCompile(`^(\S+) is (.+), line protocol is (.+?)\s*$`)
	mtuRE       = regexp.MustCompile(`MTU ([0-9]+) bytes, BW ([0-9]+) Kbit`)
	cpuRE       = regexp.MustCompile(`CPU utilization for one minute: ([0-9.]+)%; five minutes: ([0-9.]+)%; fifteen minutes: ([0-9.]+)%`)
	laneRE      = regexp.MustCompile(`^\s*([0-9]+)\s+[0-9.]+ mA\s+([-0-9.]+) dBm\s+([-0-9.]+) dBm`)
	tempRE      = regexp.MustCompile(`Temperature = ([-0-9.]+) Celsius`)
	rxErrors    = regexp.MustCompile(`([0-9]+) input errors`)
	txErrors    = regexp.MustCompile(`([0-9]+) output errors`)
	rxDrops     = regexp.MustCompile(`([0-9]+) total input drops`)
	txDrops     = regexp.MustCompile(`([0-9]+) total output drops`)
	protocolRE  = regexp.MustCompile(`Known via "([^"]+)"`)
)

func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized IOS XR output")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "%") && trim != "% Network not in table" {
			return f, fmt.Errorf("device reported: %s", trim)
		}
	}
	var err error
	switch {
	case name == "addresses4":
		err = addresses4(lines, &f)
	case name == "addresses6":
		err = addresses6(lines, &f)
	case name == "version":
		err = version(text, &f)
	case name == "route":
		err = route(lines, &f)
	case name == "lldp":
		err = lldp(lines, &f)
	case name == "cpu":
		err = cpu(text, &f)
	case strings.HasPrefix(name, "counters-"):
		err = counters(lines, &f)
	case strings.HasPrefix(name, "optics-"):
		err = optics(strings.TrimPrefix(name, "optics-"), lines, &f)
	default:
		err = fmt.Errorf("unknown IOS XR observation")
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}
func ref(i int) string { return fmt.Sprintf("line:%d", i+1) }
func addresses4(lines []string, f *driver.Facts) error {
	header := false
	for i, l := range lines {
		if strings.Contains(l, "IP-Address") && strings.Contains(l, "Vrf-Name") {
			header = true
			continue
		}
		if !header || strings.TrimSpace(l) == "" {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) != 5 {
			return fmt.Errorf("invalid IPv4 interface row")
		}
		if fields[1] == "unassigned" {
			continue
		}
		ip, e := netip.ParseAddr(fields[1])
		if e != nil || !ip.Is4() {
			return fmt.Errorf("invalid IPv4 interface address")
		}
		context := fields[4]
		f.Addresses = append(f.Addresses, driver.Address{Interface: fields[0], Address: ip.String(), RoutingContext: &context, Evidence: ref(i)})
	}
	if !header {
		return fmt.Errorf("missing IPv4 interface header")
	}
	return nil
}
func addresses6(lines []string, f *driver.Facts) error {
	iface := ""
	seen := false
	for i, l := range lines {
		line := strings.TrimSpace(l)
		if m := v6row.FindStringSubmatch(line); m != nil {
			iface = m[1]
			seen = true
			continue
		}
		if !seen || line == "" || line == "unassigned" {
			continue
		}
		ip, e := netip.ParseAddr(line)
		if e != nil || !ip.Is6() {
			return fmt.Errorf("invalid IPv6 interface address")
		}
		f.Addresses = append(f.Addresses, driver.Address{Interface: iface, Address: ip.String(), Evidence: ref(i)})
	}
	if !seen {
		return fmt.Errorf("missing IPv6 interface rows")
	}
	f.Notes = append(f.Notes, "IPv6 brief output omits prefix lengths and routing-context membership; these fields remain unknown.")
	return nil
}
func version(text string, f *driver.Facts) error {
	m := versionRE.FindStringSubmatch(text)
	if m == nil {
		return fmt.Errorf("missing IOS XR version")
	}
	f.Version = &driver.Version{OS: "Cisco IOS XR", Version: m[1], Evidence: "file"}
	if m = uptimeRE.FindStringSubmatch(text); m != nil {
		v := float64(0)
		for i, factor := range []float64{86400, 3600, 60} {
			n, e := strconv.ParseFloat(m[i+1], 64)
			if e != nil {
				return e
			}
			v += n * factor
		}
		if math.IsInf(v, 0) {
			return fmt.Errorf("uptime overflow")
		}
		f.UptimeSeconds = &v
		f.UptimeEvidence = "file"
	}
	return nil
}
func counters(lines []string, f *driver.Facts) error {
	c := driver.Counter{}
	for i, l := range lines {
		if m := interfaceRE.FindStringSubmatch(l); m != nil {
			if c.Interface != "" {
				return fmt.Errorf("multiple counter interfaces")
			}
			c.Interface, c.State, c.Evidence = m[1], m[3], ref(i)
		}
		if m := mtuRE.FindStringSubmatch(l); m != nil {
			v, e := strconv.ParseUint(m[1], 10, 64)
			if e != nil {
				return e
			}
			c.MTUBytes = &v
		}
		if m := speedRE.FindStringSubmatch(l); m != nil {
			speed, e := strconv.ParseUint(m[1], 10, 64)
			if e != nil || speed > ^uint64(0)/1000000 {
				return fmt.Errorf("invalid line speed")
			}
			speed *= 1000000
			c.SpeedBPS = &speed
		}

		for _, v := range []struct {
			pattern *regexp.Regexp
			dst     **uint64
		}{{rxErrors, &c.RXErrors}, {txErrors, &c.TXErrors}, {rxDrops, &c.RXDrops}, {txDrops, &c.TXDrops}} {
			if m := v.pattern.FindStringSubmatch(l); m != nil {
				n, e := strconv.ParseUint(m[1], 10, 64)
				if e != nil {
					return e
				}
				*v.dst = &n
			}
		}
	}
	if c.Interface == "" {
		return fmt.Errorf("missing interface counters")
	}
	f.Counters = append(f.Counters, c)
	f.Notes = append(f.Notes, "Counters are cumulative. Speed comes from the duplex/line-rate field; configurable BW is not treated as line speed. MTU framing semantics are not established by this capture, so IP PMTU comparison is withheld.")
	return nil
}
func route(lines []string, f *driver.Facts) error {
	r := driver.Route{Context: "default", NextHops: []driver.NextHop{}}
	yes := true
	for i, l := range lines {
		line := strings.TrimSpace(l)
		if line == "% Network not in table" {
			f.Notes = append(f.Notes, "No matching route was returned in the requested default context.")
			return nil
		}
		if prefix, ok := strings.CutPrefix(line, "Routing entry for "); ok {
			if r.Prefix != "" {
				return fmt.Errorf("multiple route entries")
			}
			if _, e := netip.ParsePrefix(prefix); e != nil {
				return e
			}
			r.Prefix, r.Evidence = prefix, ref(i)
		}
		if m := protocolRE.FindStringSubmatch(line); m != nil {
			r.Protocol = m[1]
		}
		if strings.HasPrefix(line, "Installed ") {
			r.Selected = &yes
		}
		if strings.HasPrefix(line, "directly connected, via ") {
			iface := strings.TrimPrefix(line, "directly connected, via ")
			if !driver.InterfaceName(iface) {
				return fmt.Errorf("invalid route interface")
			}
			r.NextHops = append(r.NextHops, driver.NextHop{Interface: iface, Active: &yes, Evidence: ref(i)})
		}
	}
	if r.Prefix == "" {
		return fmt.Errorf("missing route entry")
	}
	if r.Protocol != "connected" || len(r.NextHops) == 0 {
		return fmt.Errorf("unvalidated IOS XR route layout; only connected RIB entries are supported")
	}
	f.Routes = append(f.Routes, r)
	f.Notes = append(f.Notes, "The validated IOS XR route format is a connected RIB entry. Hardware installation and dynamic next-hop layouts remain unvalidated.")
	return nil
}
func lldp(lines []string, f *driver.Facts) error {
	var current *driver.Neighbor
	header := false
	for i, l := range lines {
		line := strings.TrimSpace(l)
		if strings.HasPrefix(line, "Total entries displayed:") {
			header = true
		}
		if iface, ok := strings.CutPrefix(line, "Local Interface: "); ok {
			header = true
			f.Neighbors = append(f.Neighbors, driver.Neighbor{Interface: iface, ManagementAddresses: []string{}, Evidence: ref(i)})
			current = &f.Neighbors[len(f.Neighbors)-1]
			continue
		}
		if current == nil {
			continue
		}
		for prefix, dst := range map[string]*string{"Chassis id: ": &current.ChassisID, "Port id: ": &current.PortID, "System Name: ": &current.Name} {
			if v, ok := strings.CutPrefix(line, prefix); ok {
				*dst = v
			}
		}
		for _, prefix := range []string{"IPv4 address: ", "IPv6 address: "} {
			if v, ok := strings.CutPrefix(line, prefix); ok {
				a, e := netip.ParseAddr(v)
				if e != nil {
					return e
				}
				current.ManagementAddresses = append(current.ManagementAddresses, a.Unmap().String())
			}
		}
	}
	if !header {
		return fmt.Errorf("missing LLDP result")
	}
	return nil
}
func cpu(text string, f *driver.Facts) error {
	matches := cpuRE.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		return fmt.Errorf("expected one CPU summary; multiple nodes need a separate representation")
	}
	values := make([]float64, 3)
	for i := range values {
		v, e := strconv.ParseFloat(matches[0][i+1], 64)
		if e != nil || v < 0 || v > 100 {
			return fmt.Errorf("invalid CPU utilization")
		}
		values[i] = v
	}
	f.CPU = &driver.CPU{Utilization1: &values[0], Utilization5: &values[1], Utilization15: &values[2], Evidence: "file"}
	return nil
}
func optics(iface string, lines []string, f *driver.Facts) error {
	var temp *float64
	seen := false
	for _, l := range lines {
		if strings.Contains(l, "Optics Status") {
			seen = true
		}
		if m := tempRE.FindStringSubmatch(l); m != nil {
			v, e := strconv.ParseFloat(m[1], 64)
			if e != nil {
				return e
			}
			temp = &v
		}
	}
	if !seen {
		return fmt.Errorf("missing optics status")
	}
	for i, l := range lines {
		if m := laneRE.FindStringSubmatch(l); m != nil {
			tx, e := strconv.ParseFloat(m[2], 64)
			if e != nil {
				return e
			}
			rx, e := strconv.ParseFloat(m[3], 64)
			if e != nil {
				return e
			}
			f.Optics = append(f.Optics, driver.Optical{Interface: iface, Lane: m[1], TemperatureCelsius: temp, TXPowerDBM: &tx, RXPowerDBM: &rx, Evidence: ref(i)})
		}
	}
	if len(f.Optics) == 0 {
		f.Notes = append(f.Notes, "No supported per-lane DOM measurements were returned.")
	}
	return nil
}
