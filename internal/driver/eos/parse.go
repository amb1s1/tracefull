package eos

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized EOS output")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return f, fmt.Errorf("invalid EOS JSON object")
	}
	var err error
	switch {
	case name == "cpu":
		err = cpu(data, &f)
	case name == "version":
		err = version(data, &f)
	case name == "route":
		err = route(data, &f)
	case name == "lldp":
		err = lldp(data, &f)
	case name == "addresses4" || name == "addresses6" || strings.HasPrefix(name, "counters-") || strings.HasPrefix(name, "optics-"):
		var rows map[string]json.RawMessage
		if err = json.Unmarshal(doc["interfaces"], &rows); err == nil {
			if rows == nil {
				err = fmt.Errorf("missing interfaces object")
			} else {
				err = interfaces(name, rows, &f)
			}
		}
	default:
		err = fmt.Errorf("unknown EOS observation")
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

type ip4 struct {
	Address string
	MaskLen *int
}

func interfaces(name string, rows map[string]json.RawMessage, f *driver.Facts) error {
	for _, iface := range keys(rows) {
		ref := "json:/interfaces/" + driver.JSONPart(iface)
		var r struct {
			Name, VRF, LineProtocolStatus string
			MTU, Bandwidth                *uint64
			Addresses                     []struct{ Address, Subnet string }
			LinkLocal                     struct{ Address, Subnet string }
			InterfaceAddress              json.RawMessage
			InterfaceCounters             struct{ TotalInErrors, TotalOutErrors, InDiscards, OutDiscards *uint64 }
			Temperature, TXPower, RXPower *float64
			Channel                       string
		}
		if err := json.Unmarshal(rows[iface], &r); err != nil {
			return err
		}
		if strings.HasPrefix(name, "counters-") {
			// EOS reports zero bandwidth for an SVI; it has no physical line rate.
			var speed *uint64
			if (strings.HasPrefix(iface, "Ethernet") || strings.HasPrefix(iface, "Management")) && r.Bandwidth != nil && *r.Bandwidth > 0 {
				speed = r.Bandwidth
			}
			f.Counters = append(f.Counters, driver.Counter{Interface: iface, State: r.LineProtocolStatus, MTUBytes: r.MTU, SpeedBPS: speed, RXErrors: r.InterfaceCounters.TotalInErrors, TXErrors: r.InterfaceCounters.TotalOutErrors, RXDrops: r.InterfaceCounters.InDiscards, TXDrops: r.InterfaceCounters.OutDiscards, Evidence: ref})
			continue
		}
		if strings.HasPrefix(name, "optics-") {
			f.Optics = append(f.Optics, driver.Optical{Interface: iface, Lane: r.Channel, TemperatureCelsius: r.Temperature, TXPowerDBM: r.TXPower, RXPowerDBM: r.RXPower, Evidence: ref})
			continue
		}
		context := (*string)(nil)
		if r.VRF != "" {
			context = &r.VRF
		}
		add := func(ip string, prefix *int, where string) error {
			if ip == "" {
				return nil
			}
			a, e := netip.ParseAddr(ip)
			if e != nil {
				return e
			}
			if a.IsUnspecified() {
				return nil
			}
			if prefix != nil && (*prefix < 0 || *prefix > a.BitLen()) {
				return fmt.Errorf("invalid interface prefix")
			}
			f.Addresses = append(f.Addresses, driver.Address{Interface: iface, Address: a.Unmap().String(), PrefixLength: prefix, RoutingContext: context, Evidence: where})
			return nil
		}
		if name == "addresses4" {
			// Counter output uses an array for this field; IPv4 interface output
			// uses an object. Decode only the selected command's dialect.
			var address struct {
				PrimaryIP           ip4
				SecondaryIPs        map[string]ip4
				VirtualIP           ip4
				VirtualSecondaryIPs map[string]ip4
			}
			if len(r.InterfaceAddress) > 0 {
				if err := json.Unmarshal(r.InterfaceAddress, &address); err != nil {
					return err
				}
			}
			for _, a := range []struct {
				v   ip4
				ref string
			}{{address.PrimaryIP, ref + "/interfaceAddress/primaryIp"}, {address.VirtualIP, ref + "/interfaceAddress/virtualIp"}} {
				if err := add(a.v.Address, a.v.MaskLen, a.ref); err != nil {
					return err
				}
			}
			for _, list := range []struct {
				m    map[string]ip4
				path string
			}{{address.SecondaryIPs, "secondaryIps"}, {address.VirtualSecondaryIPs, "virtualSecondaryIps"}} {
				for _, key := range keys(list.m) {
					v := list.m[key]
					if v.Address == "" {
						v.Address = key
					}
					if err := add(v.Address, v.MaskLen, ref+"/interfaceAddress/"+list.path+"/"+driver.JSONPart(key)); err != nil {
						return err
					}
				}
			}
		} else {
			for i, a := range r.Addresses {
				prefix, e := netip.ParsePrefix(a.Subnet)
				if e != nil {
					return e
				}
				bits := prefix.Bits()
				if err := add(a.Address, &bits, fmt.Sprintf("%s/addresses/%d", ref, i)); err != nil {
					return err
				}
			}
			if r.LinkLocal.Address != "" {
				var bits *int
				if r.LinkLocal.Subnet != "" {
					p, e := netip.ParsePrefix(r.LinkLocal.Subnet)
					if e != nil {
						return e
					}
					n := p.Bits()
					bits = &n
				}
				if err := add(r.LinkLocal.Address, bits, ref+"/linkLocal"); err != nil {
					return err
				}
			}
		}
	}
	if strings.HasPrefix(name, "counters-") {
		f.Notes = append(f.Notes, "Counters are cumulative snapshots, not incident rates.")
	}
	if strings.HasPrefix(name, "optics-") && len(f.Optics) == 0 {
		f.Notes = append(f.Notes, "No DOM measurements were returned; optics values remain unknown.")
	}
	return nil
}
func version(data []byte, f *driver.Facts) error {
	var r struct {
		Version, MfgName string
		Uptime           *float64
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	if r.MfgName != "Arista" || r.Version == "" {
		return fmt.Errorf("not an EOS version document")
	}
	if r.Uptime != nil && *r.Uptime < 0 {
		return fmt.Errorf("negative uptime")
	}
	f.Version = &driver.Version{OS: "Arista EOS", Version: r.Version, Evidence: "json:/version"}
	f.UptimeSeconds = r.Uptime
	f.UptimeEvidence = "json:/uptime"
	return nil
}
func route(data []byte, f *driver.Facts) error {
	var doc struct {
		VRFs map[string]struct {
			Routes map[string]struct {
				RouteType, RouteAction               string
				HardwareProgrammed, KernelProgrammed *bool
				Vias                                 []struct{ NexthopAddr, Interface string }
			}
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.VRFs == nil {
		return fmt.Errorf("missing VRF route object")
	}
	for _, vrf := range keys(doc.VRFs) {
		for _, prefix := range keys(doc.VRFs[vrf].Routes) {
			if _, e := netip.ParsePrefix(prefix); e != nil {
				return e
			}
			row := doc.VRFs[vrf].Routes[prefix]
			ref := "json:/vrfs/" + driver.JSONPart(vrf) + "/routes/" + driver.JSONPart(prefix)
			r := driver.Route{Prefix: prefix, Protocol: row.RouteType, Context: vrf, Installed: row.HardwareProgrammed, NextHops: []driver.NextHop{}, Evidence: ref}
			if row.RouteAction != "" {
				selected := row.RouteAction == "forward"
				r.Selected = &selected
			}
			for i, n := range row.Vias {
				if n.NexthopAddr != "" {
					if _, e := netip.ParseAddr(n.NexthopAddr); e != nil {
						return e
					}
				}
				r.NextHops = append(r.NextHops, driver.NextHop{Address: n.NexthopAddr, Interface: n.Interface, Active: r.Selected, Installed: row.HardwareProgrammed, Evidence: fmt.Sprintf("%s/vias/%d", ref, i)})
			}
			f.Routes = append(f.Routes, r)
		}
	}
	f.Notes = append(f.Notes, "Route hardware-programmed state is a later snapshot, not proof of the saved trace's forwarding path.")
	return nil
}
func lldp(data []byte, f *driver.Facts) error {
	var doc struct {
		LLDPNeighbors map[string]struct {
			LLDPNeighborInfo []struct {
				ChassisID, SystemName string
				ManagementAddresses   []struct{ Address string }
				NeighborInterfaceInfo struct {
					InterfaceIDV2 string `json:"interfaceId_v2"`
					InterfaceID   string
				}
			}
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.LLDPNeighbors == nil {
		return fmt.Errorf("missing LLDP object")
	}
	for _, iface := range keys(doc.LLDPNeighbors) {
		for i, row := range doc.LLDPNeighbors[iface].LLDPNeighborInfo {
			n := driver.Neighbor{Interface: iface, Name: row.SystemName, ChassisID: row.ChassisID, PortID: row.NeighborInterfaceInfo.InterfaceIDV2, ManagementAddresses: []string{}, Evidence: fmt.Sprintf("json:/lldpNeighbors/%s/lldpNeighborInfo/%d", driver.JSONPart(iface), i)}
			if n.PortID == "" {
				n.PortID = row.NeighborInterfaceInfo.InterfaceID
			}
			for _, a := range row.ManagementAddresses {
				ip, e := netip.ParseAddr(a.Address)
				if e != nil {
					return e
				}
				n.ManagementAddresses = append(n.ManagementAddresses, ip.Unmap().String())
			}
			f.Neighbors = append(f.Neighbors, n)
		}
	}
	return nil
}

func cpu(data []byte, f *driver.Facts) error {
	var doc struct {
		CPUInfo  map[string]struct{ Idle *float64 }
		TimeInfo struct{ LoadAvg []float64 }
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.CPUInfo == nil {
		return fmt.Errorf("missing CPU information")
	}
	c := &driver.CPU{Evidence: "json:/cpuInfo"}
	if idle := doc.CPUInfo["%Cpu(s)"].Idle; idle != nil {
		if *idle < 0 || *idle > 100 {
			return fmt.Errorf("invalid CPU idle percent")
		}
		value := 100 - *idle
		c.Utilization = &value
	}
	if len(doc.TimeInfo.LoadAvg) > 0 {
		if len(doc.TimeInfo.LoadAvg) != 3 {
			return fmt.Errorf("expected three load averages")
		}
		for _, v := range doc.TimeInfo.LoadAvg {
			if v < 0 {
				return fmt.Errorf("negative CPU load")
			}
		}
		c.Load1, c.Load5, c.Load15 = &doc.TimeInfo.LoadAvg[0], &doc.TimeInfo.LoadAvg[1], &doc.TimeInfo.LoadAvg[2]
		c.Evidence = "file"
	}
	f.CPU = c
	f.Notes = append(f.Notes, "Reported CPU utilization is 100 minus idle; its sampling interval is unspecified and is not assigned to a 1/5/15-minute window. Load averages are separate observations.")
	return nil
}
