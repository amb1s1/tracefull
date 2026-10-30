package junos

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

type node map[string]json.RawMessage

func (n node) list(k string) []node { var v []node; _ = json.Unmarshal(n[k], &v); return v }
func (n node) one(k string) node {
	v := n.list(k)
	if len(v) == 1 {
		return v[0]
	}
	return nil
}
func (n node) value(k string) string {
	var s string
	_ = json.Unmarshal(n.one(k)["data"], &s)
	return s
}
func (n node) attribute(k, a string) string {
	var attrs map[string]string
	_ = json.Unmarshal(n.one(k)["attributes"], &attrs)
	return attrs[a]
}
func number(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("invalid numeric field %q", s)
	}
	return &v, nil
}
func count(s string) (*uint64, error) {
	if s == "" || s == "Unspecified" || s == "Unlimited" {
		return nil, nil
	}
	v, e := strconv.ParseUint(s, 10, 64)
	return &v, e
}
func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized Junos output")
	}
	var root node
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return f, fmt.Errorf("invalid Junos JSON")
	}
	if err := validateNode(root, 0); err != nil {
		return f, err
	}
	if _, ok := root["rpc-error"]; ok {
		return f, fmt.Errorf("junos RPC error")
	}
	var err error
	switch {
	case name == "addresses":
		err = addresses(root, &f)
	case name == "route":
		err = routes(root, &f)
	case name == "version":
		n := root.one("software-information")
		v := n.value("junos-version")
		if v == "" {
			err = fmt.Errorf("missing Junos version")
		} else {
			f.Version = &driver.Version{OS: "Junos", Version: v, Evidence: "json:/software-information/0/junos-version/0"}
		}
	case name == "uptime":
		n := root.one("system-uptime-information")
		if n == nil {
			err = fmt.Errorf("missing uptime object")
		} else {
			f.UptimeSeconds, err = number(n.one("system-booted-time").attribute("time-length", "junos:seconds"))
			f.UptimeEvidence = "json:/system-uptime-information/0/system-booted-time/0/time-length/0"
			if f.UptimeSeconds != nil && *f.UptimeSeconds < 0 {
				err = fmt.Errorf("negative uptime")
			}
		}
	case name == "cpu":
		err = cpu(root, &f)
	case name == "lldp":
		err = lldp(root, &f)
	case strings.HasPrefix(name, "counters-"):
		err = counters(root, &f)
	case strings.HasPrefix(name, "optics-"):
		err = optics(root, &f)
	default:
		err = fmt.Errorf("unknown Junos observation")
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}
func required(root node, key string) ([]node, error) {
	v, ok := root[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	var rows []node
	if err := json.Unmarshal(v, &rows); err != nil || rows == nil {
		return nil, fmt.Errorf("invalid %s array", key)
	}
	return rows, nil
}
func addresses(root node, f *driver.Facts) error {
	groups, e := required(root, "interface-information")
	if e != nil {
		return e
	}
	for gi, g := range groups {
		for pi, p := range g.list("physical-interface") {
			for li, l := range p.list("logical-interface") {
				iface := l.value("name")
				if iface == "" {
					return fmt.Errorf("missing logical interface")
				}
				for ai, af := range l.list("address-family") {
					if af.value("address-family-name") != "inet" && af.value("address-family-name") != "inet6" {
						continue
					}
					for ii, a := range af.list("interface-address") {
						raw := a.value("ifa-local")
						if raw == "" {
							continue
						}
						ip, err := netip.ParseAddr(raw)
						var bits *int
						if prefix, e := netip.ParsePrefix(raw); e == nil {
							ip = prefix.Addr()
							n := prefix.Bits()
							bits = &n
							err = nil
						}
						if err != nil {
							return err
						}
						f.Addresses = append(f.Addresses, driver.Address{Interface: iface, Address: ip.Unmap().String(), PrefixLength: bits, Evidence: fmt.Sprintf("json:/interface-information/%d/physical-interface/%d/logical-interface/%d/address-family/%d/interface-address/%d", gi, pi, li, ai, ii)})
					}
				}
			}
		}
	}
	f.Notes = append(f.Notes, "Terse interface output does not identify routing-instance membership; address routing context remains unknown.")
	return nil
}
func routes(root node, f *driver.Facts) error {
	groups, e := required(root, "route-information")
	if e != nil {
		return e
	}
	for gi, g := range groups {
		for ti, t := range g.list("route-table") {
			table := t.value("table-name")
			context := table
			if table == "inet.0" || table == "inet6.0" {
				context = "default"
			}
			for ri, r := range t.list("rt") {
				prefix := r.value("rt-destination")
				if !strings.Contains(prefix, "/") {
					prefix += "/" + r.value("rt-prefix-length")
				}
				if _, err := netip.ParsePrefix(prefix); err != nil {
					return err
				}
				for ei, entry := range r.list("rt-entry") {
					ref := fmt.Sprintf("json:/route-information/%d/route-table/%d/rt/%d/rt-entry/%d", gi, ti, ri, ei)
					route := driver.Route{Prefix: prefix, Protocol: entry.value("protocol-name"), Context: context, NextHops: []driver.NextHop{}, Evidence: ref}
					if tag := entry.value("active-tag"); tag != "" {
						v := tag == "*" || tag == "+"
						route.Selected = &v
					}
					for ni, nh := range entry.list("nh") {
						ip := nh.value("to")
						if ip != "" {
							if _, err := netip.ParseAddr(ip); err != nil {
								return err
							}
						}
						n := driver.NextHop{Address: ip, Interface: nh.value("via"), Evidence: fmt.Sprintf("%s/nh/%d", ref, ni)}
						if len(nh.list("selected-next-hop")) == 1 {
							yes := true
							n.Active = &yes
						}
						route.NextHops = append(route.NextHops, n)
					}
					f.Routes = append(f.Routes, route)
				}
			}
		}
	}
	f.Notes = append(f.Notes, "Active RIB routes and selected next hops are reported; hardware installation is unknown without a forwarding-table observation.")
	return nil
}
func counters(root node, f *driver.Facts) error {
	groups, e := required(root, "interface-information")
	if e != nil {
		return e
	}
	for gi, g := range groups {
		for _, kind := range []string{"physical-interface", "logical-interface"} {
			for ii, n := range g.list(kind) {
				iface := n.value("name")
				if iface == "" {
					return fmt.Errorf("missing counter interface")
				}
				c := driver.Counter{Interface: iface, State: n.value("oper-status"), Evidence: fmt.Sprintf("json:/interface-information/%d/%s/%d", gi, kind, ii)}
				var err error
				c.MTUBytes, err = count(n.value("mtu"))
				if err != nil {
					return err
				}
				for _, item := range []struct {
					dst   **uint64
					value string
				}{{&c.RXErrors, n.one("input-error-list").value("input-errors")}, {&c.RXDrops, n.one("input-error-list").value("input-drops")}, {&c.TXErrors, n.one("output-error-list").value("output-errors")}, {&c.TXDrops, n.one("output-error-list").value("output-drops")}} {
					*item.dst, err = count(item.value)
					if err != nil {
						return err
					}
				}
				// Physical Junos MTU includes link framing; keep it out of IP PMTU comparisons.
				c.MTULayer = "link"
				if speed := n.value("speed"); strings.HasSuffix(speed, "Gbps") {
					v, err := strconv.ParseUint(strings.TrimSuffix(speed, "Gbps"), 10, 64)
					if err != nil || v > ^uint64(0)/1000000000 {
						return fmt.Errorf("invalid link speed")
					}
					v *= 1000000000
					c.SpeedBPS = &v
				}
				f.Counters = append(f.Counters, c)
			}
		}
	}
	f.Notes = append(f.Notes, "Junos physical MTU includes link framing; it is not directly comparable to IP PMTU. Counters are cumulative; unreported logical-interface counters remain unknown.")
	return nil
}
func cpu(root node, f *driver.Facts) error {
	groups, e := required(root, "route-engine-information")
	if e != nil {
		return e
	}
	masters := 0
	for gi, g := range groups {
		for ri, r := range g.list("route-engine") {
			if r.value("mastership-state") != "master" {
				continue
			}
			masters++
			c := &driver.CPU{Evidence: fmt.Sprintf("json:/route-engine-information/%d/route-engine/%d", gi, ri)}
			for _, v := range []struct {
				dst  **float64
				name string
			}{{&c.Load1, "load-average-one"}, {&c.Load5, "load-average-five"}, {&c.Load15, "load-average-fifteen"}} {
				var err error
				*v.dst, err = number(r.value(v.name))
				if err != nil {
					return err
				}
				if *v.dst != nil && **v.dst < 0 {
					return fmt.Errorf("negative load")
				}
			}
			f.CPU = c
		}
	}
	if masters != 1 {
		f.CPU = nil
		f.Notes = append(f.Notes, "A unique master routing engine was not identified; aggregate CPU load is unknown.")
	}
	return nil
}
func lldp(root node, f *driver.Facts) error {
	groups, e := required(root, "lldp-neighbors-information")
	if e != nil {
		return e
	}
	for gi, g := range groups {
		for ni, n := range g.list("lldp-neighbor-information") {
			iface := n.value("lldp-local-port-id")
			if iface == "" {
				return fmt.Errorf("missing LLDP local port")
			}
			f.Neighbors = append(f.Neighbors, driver.Neighbor{Interface: iface, Name: n.value("lldp-remote-system-name"), ChassisID: n.value("lldp-remote-chassis-id"), PortID: n.value("lldp-remote-port-id"), ManagementAddresses: []string{}, Evidence: fmt.Sprintf("json:/lldp-neighbors-information/%d/lldp-neighbor-information/%d", gi, ni)})
		}
	}
	return nil
}
func optics(root node, f *driver.Facts) error {
	groups, e := required(root, "interface-information")
	if e != nil {
		return e
	}
	for gi, g := range groups {
		for pi, p := range g.list("physical-interface") {
			for oi, o := range p.list("optics-diagnostics") {
				temp, err := number(o.attribute("module-temperature", "junos:celsius"))
				if err != nil {
					return err
				}
				for li, l := range o.list("optics-diagnostics-lane-values") {
					tx, err := number(l.value("laser-output-power-dbm"))
					if err != nil {
						return err
					}
					rx, err := number(l.value("laser-rx-optical-power-dbm"))
					if err != nil {
						return err
					}
					f.Optics = append(f.Optics, driver.Optical{Interface: p.value("name"), Lane: l.value("lane-index"), TemperatureCelsius: temp, TXPowerDBM: tx, RXPowerDBM: rx, Evidence: fmt.Sprintf("json:/interface-information/%d/physical-interface/%d/optics-diagnostics/%d/optics-diagnostics-lane-values/%d", gi, pi, oi, li)})
				}
			}
		}
	}
	if len(f.Optics) == 0 {
		f.Notes = append(f.Notes, "No supported per-lane DOM measurements were returned; values remain unknown.")
	}
	return nil
}

// Junos XML-to-JSON uses arrays of nodes, string data and string attributes.
// Reject malformed containers before optional-field helpers interpret absence.
func validateNode(n node, depth int) error {
	if n == nil || depth > 64 {
		return fmt.Errorf("invalid or deeply nested Junos node")
	}
	for key, raw := range n {
		switch key {
		case "data":
			var s string
			if string(raw) == "null" || json.Unmarshal(raw, &s) != nil {
				// Empty XML flags are encoded as data:[null], not as strings.
				var flag []*string
				if json.Unmarshal(raw, &flag) != nil || len(flag) != 1 || flag[0] != nil {
					return fmt.Errorf("invalid Junos data")
				}
			}
		case "attributes":
			var attrs map[string]string
			if json.Unmarshal(raw, &attrs) != nil || attrs == nil {
				return fmt.Errorf("invalid Junos attributes")
			}
		default:
			var rows []node
			if json.Unmarshal(raw, &rows) != nil || rows == nil {
				return fmt.Errorf("invalid Junos array %q", key)
			}
			for _, row := range rows {
				if err := validateNode(row, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
