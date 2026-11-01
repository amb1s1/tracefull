package nxos

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	var obj map[string]json.RawMessage
	if len(data) == 0 || len(data) > 1<<20 || json.Unmarshal(data, &obj) != nil || obj == nil {
		return f, fmt.Errorf("invalid or oversized NX-OS JSON object")
	}
	n := node{data, "json:"}
	var err error
	switch {
	case name == "version":
		err = version(n, &f)
	case name == "cpu":
		err = cpu(n, &f)
	case name == "addresses4" || name == "addresses6":
		err = addresses(name, n, &f)
	case name == "route":
		err = routes(n, &f)
	case name == "lldp":
		err = neighbors(n, &f)
	case strings.HasPrefix(name, "counters-"):
		err = counters(n, &f)
	case strings.HasPrefix(name, "optics-"):
		err = optics(n, &f)
	default:
		err = fmt.Errorf("unknown NX-OS observation")
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}
func version(n node, f *driver.Facts) error {
	var v struct {
		Version string `json:"nxos_ver_str"`
		Header  string `json:"header_str"`
		Days    string `json:"kern_uptm_days"`
		Hours   string `json:"kern_uptm_hrs"`
		Minutes string `json:"kern_uptm_mins"`
		Seconds string `json:"kern_uptm_secs"`
	}
	if err := json.Unmarshal(n.data, &v); err != nil {
		return err
	}
	if v.Version == "" || !strings.Contains(v.Header, "NX-OS") {
		return fmt.Errorf("missing NX-OS identity")
	}
	f.Version = &driver.Version{OS: "Cisco NX-OS", Version: v.Version, Evidence: "json:/nxos_ver_str"}
	var total uint64
	known := true
	for i, s := range []string{v.Days, v.Hours, v.Minutes, v.Seconds} {
		x, e := uintValue(s)
		if e != nil {
			return e
		}
		if x == nil {
			known = false
			continue
		}
		factor := []uint64{86400, 3600, 60, 1}[i]
		if *x > (^uint64(0)-total)/factor {
			return fmt.Errorf("uptime overflow")
		}
		total += *x * factor
	}
	if known {
		x := float64(total)
		f.UptimeSeconds = &x
		f.UptimeEvidence = "json:/kern_uptm_days"
	}
	return nil
}
func cpu(n node, f *driver.Facts) error {
	var v struct {
		Idle   string `json:"cpu_state_idle"`
		Load1  string `json:"load_avg_1min"`
		Load5  string `json:"load_avg_5min"`
		Load15 string `json:"load_avg_15min"`
	}
	if err := json.Unmarshal(n.data, &v); err != nil {
		return err
	}
	if v.Idle == "" && v.Load1 == "" && v.Load5 == "" && v.Load15 == "" {
		return fmt.Errorf("missing CPU measurements")
	}
	c := &driver.CPU{Evidence: "json:"}
	var err error
	if c.Utilization, err = floatValue(v.Idle); err != nil {
		return err
	}
	if c.Utilization != nil {
		if *c.Utilization < 0 || *c.Utilization > 100 {
			return fmt.Errorf("invalid idle percentage")
		}
		*c.Utilization = 100 - *c.Utilization
	}
	for _, x := range []struct {
		s string
		p **float64
	}{{v.Load1, &c.Load1}, {v.Load5, &c.Load5}, {v.Load15, &c.Load15}} {
		if *x.p, err = floatValue(x.s); err != nil {
			return err
		}
		if *x.p != nil && **x.p < 0 {
			return fmt.Errorf("negative load")
		}
	}
	f.CPU = c
	return nil
}
func addresses(name string, n node, f *driver.Facts) error {
	rows, err := children(n, "TABLE_intf", "ROW_intf", false)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var v struct {
			Interface string `json:"intf-name"`
			Context   string `json:"vrf-name-out"`
			Prefix    string `json:"prefix"`
			LinkLocal string `json:"linklocal-addr"`
		}
		if err := json.Unmarshal(r.data, &v); err != nil {
			return err
		}
		if !driver.InterfaceName(v.Interface) || v.Context == "" {
			return fmt.Errorf("missing interface/context")
		}
		for _, s := range []string{v.Prefix, v.LinkLocal} {
			if s == "" {
				continue
			}
			a, e := netip.ParseAddr(s)
			if e != nil || a.Zone() != "" || (name == "addresses4") != a.Is4() {
				return fmt.Errorf("invalid interface address")
			}
			context := v.Context
			f.Addresses = append(f.Addresses, driver.Address{Interface: v.Interface, Address: a.String(), RoutingContext: &context, Evidence: r.ref})
		}
	}
	f.Notes = append(f.Notes, "All-VRF address reads retain membership; brief output does not establish prefix lengths or the saved trace's routing context.")
	return nil
}
func counters(n node, f *driver.Facts) error {
	rows, err := children(n, "TABLE_interface", "ROW_interface", false)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var v struct {
			Interface, State string
			MTU              string `json:"eth_mtu"`
			Speed            string `json:"eth_speed"`
			RXErrors         string `json:"eth_inerr"`
			TXErrors         string `json:"eth_outerr"`
			RXDrops          string `json:"eth_indiscard"`
			TXDrops          string `json:"eth_outdiscard"`
		}
		if err := json.Unmarshal(r.data, &v); err != nil {
			return err
		}
		if !driver.InterfaceName(v.Interface) {
			return fmt.Errorf("missing counter interface")
		}
		c := driver.Counter{Interface: v.Interface, State: v.State, Evidence: r.ref}
		for _, x := range []struct {
			s string
			p **uint64
		}{{v.MTU, &c.MTUBytes}, {v.RXErrors, &c.RXErrors}, {v.TXErrors, &c.TXErrors}, {v.RXDrops, &c.RXDrops}, {v.TXDrops, &c.TXDrops}} {
			if *x.p, err = uintValue(x.s); err != nil {
				return err
			}
		}
		if v.Speed != "" {
			parts := strings.Fields(v.Speed)
			if len(parts) != 2 {
				return fmt.Errorf("unsupported speed")
			}
			factor := uint64(0)
			switch parts[1] {
			case "Mb/s":
				factor = 1000000
			case "Gb/s":
				factor = 1000000000
			default:
				return fmt.Errorf("unsupported speed unit")
			}
			x, e := uintValue(parts[0])
			if e != nil || x == nil || *x > ^uint64(0)/factor {
				return fmt.Errorf("invalid speed")
			}
			*x *= factor
			c.SpeedBPS = x
		}
		f.Counters = append(f.Counters, c)
	}
	f.Notes = append(f.Notes, "Counters are cumulative; configurable bandwidth is not line speed. MTU framing is unvalidated and cannot establish IP PMTU.")
	return nil
}
func optics(n node, f *driver.Facts) error {
	rows, err := children(n, "TABLE_interface", "ROW_interface", false)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var v struct{ Interface, SFP string }
		if err := json.Unmarshal(r.data, &v); err != nil {
			return err
		}
		if !driver.InterfaceName(v.Interface) {
			return fmt.Errorf("missing optical interface")
		}
		lanes, e := children(r, "TABLE_lane", "ROW_lane", true)
		if e != nil {
			return e
		}
		if len(lanes) > 1 {
			return fmt.Errorf("multi-lane layout not capture-validated")
		}
		for _, lane := range lanes {
			var x struct {
				Temperature string
				TX          string `json:"tx_pwr"`
				RX          string `json:"rx_pwr"`
			}
			if err := json.Unmarshal(lane.data, &x); err != nil {
				return err
			}
			o := driver.Optical{Interface: v.Interface, Evidence: lane.ref}
			if o.TemperatureCelsius, err = floatValue(x.Temperature); err != nil {
				return err
			}
			if o.TXPowerDBM, err = floatValue(x.TX); err != nil {
				return err
			}
			if o.RXPowerDBM, err = floatValue(x.RX); err != nil {
				return err
			}
			f.Optics = append(f.Optics, o)
		}
	}
	f.Notes = append(f.Notes, "Only the captured single-lane optical layout is validated; no lane identifier or optic health is inferred.")
	return nil
}
func neighbors(n node, f *driver.Facts) error {
	rows, err := children(n, "TABLE_nbor_detail", "ROW_nbor_detail", false)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var v struct {
			Interface string `json:"l_port_id"`
			Name      string `json:"sys_name"`
			Chassis   string `json:"chassis_id"`
			Port      string `json:"port_id"`
			IP        string `json:"mgmt_addr"`
			IPv6      string `json:"mgmt_addr_ipv6"`
		}
		if err := json.Unmarshal(r.data, &v); err != nil {
			return err
		}
		if !driver.InterfaceName(v.Interface) {
			return fmt.Errorf("missing LLDP local interface")
		}
		iface := v.Interface
		if strings.HasPrefix(iface, "Eth") && !strings.HasPrefix(iface, "Ethernet") {
			iface = "Ethernet" + strings.TrimPrefix(iface, "Eth")
		}
		neighbor := driver.Neighbor{Interface: iface, Name: v.Name, ChassisID: v.Chassis, PortID: v.Port, ManagementAddresses: []string{}, Evidence: r.ref}
		for _, s := range []string{v.IP, v.IPv6} {
			if s == "" || s == "not advertised" {
				continue
			}
			a, e := netip.ParseAddr(s)
			if e != nil {
				return e
			}
			neighbor.ManagementAddresses = append(neighbor.ManagementAddresses, a.String())
		}
		f.Neighbors = append(f.Neighbors, neighbor)
	}
	return nil
}
