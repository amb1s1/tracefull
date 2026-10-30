package cumulus

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized device output")
	}
	var err error
	switch {
	case name == "addresses":
		err = parseAddresses(data, &f)
	case name == "route":
		err = parseRoute(data, &f)
	case strings.HasPrefix(name, "counters-"):
		err = parseCounters(data, &f)
	case name == "lldp":
		err = parseLLDP(data, &f)
	case name == "version":
		err = parseVersion(data, &f)
	case name == "uptime":
		var values []float64
		values, err = numbers(data, 2)
		if err == nil {
			f.UptimeSeconds = &values[0]
			f.UptimeEvidence = "line:1"
		}
	case name == "cpu":
		var values []float64
		values, err = numbers(data, 3)
		if err == nil {
			f.CPU = &driver.CPU{Load1: &values[0], Load5: &values[1], Load15: &values[2], Evidence: "line:1"}
			f.Notes = append(f.Notes, "Load averages are not CPU utilization percentages.")
		}
	default:
		err = fmt.Errorf("unsupported Cumulus observation %q", name)
	}
	if err != nil {
		return driver.Empty(), fmt.Errorf("parse %s: %w", name, err)
	}
	return f, nil
}

type link struct {
	Name      string  `json:"ifname"`
	Master    string  `json:"master"`
	MTU       *uint64 `json:"mtu"`
	State     string  `json:"operstate"`
	Addresses []struct {
		Local        string `json:"local"`
		PrefixLength *int   `json:"prefixlen"`
	} `json:"addr_info"`
	Stats struct {
		RX struct{ Errors, Dropped *uint64 } `json:"rx"`
		TX struct{ Errors, Dropped *uint64 } `json:"tx"`
	} `json:"stats64"`
}

func links(data []byte) ([]link, error) {
	var rows []link
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, fmt.Errorf("expected link array")
	}
	for _, row := range rows {
		if row.Name == "" {
			return nil, fmt.Errorf("link lacks interface name")
		}
	}
	return rows, nil
}
func parseAddresses(data []byte, f *driver.Facts) error {
	rows, err := links(data)
	if err != nil {
		return err
	}
	for i, row := range rows {
		for j, addr := range row.Addresses {
			a, err := netip.ParseAddr(addr.Local)
			if err != nil {
				return fmt.Errorf("invalid interface address")
			}
			if addr.PrefixLength != nil && (*addr.PrefixLength < 0 || *addr.PrefixLength > a.BitLen()) {
				return fmt.Errorf("invalid interface prefix length")
			}
			var routingContext *string
			if row.Master == "" {
				v := "default"
				routingContext = &v
			}
			f.Addresses = append(f.Addresses, driver.Address{RoutingContext: routingContext, Interface: row.Name, Address: a.Unmap().String(), PrefixLength: addr.PrefixLength, Master: row.Master, Evidence: fmt.Sprintf("json:/%d/addr_info/%d", i, j)})
		}
	}
	return nil
}
func parseCounters(data []byte, f *driver.Facts) error {
	rows, err := links(data)
	if err != nil {
		return err
	}
	for i, row := range rows {
		f.Counters = append(f.Counters, driver.Counter{MTULayer: "ip", Interface: row.Name, State: row.State, MTUBytes: row.MTU, RXErrors: row.Stats.RX.Errors, TXErrors: row.Stats.TX.Errors, RXDrops: row.Stats.RX.Dropped, TXDrops: row.Stats.TX.Dropped, Evidence: fmt.Sprintf("json:/%d", i)})
	}
	f.Notes = append(f.Notes, "Counters are cumulative snapshots, not rates or evidence of an incident time. Speed and CRC-specific counters are unknown in this capture format.")
	return nil
}
func pointerPart(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }
func parseRoute(data []byte, f *driver.Facts) error {
	var routes map[string][]struct {
		Protocol  string `json:"protocol"`
		VRF       string `json:"vrfName"`
		Selected  *bool  `json:"selected"`
		Installed *bool  `json:"installed"`
		NextHops  []struct {
			IP        string `json:"ip"`
			Interface string `json:"interfaceName"`
			Active    *bool  `json:"active"`
			FIB       *bool  `json:"fib"`
		} `json:"nexthops"`
	}
	if err := json.Unmarshal(data, &routes); err != nil {
		return err
	}
	if routes == nil {
		return fmt.Errorf("expected FRR route object")
	}
	keys := make([]string, 0, len(routes))
	for key := range routes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, prefix := range keys {
		if _, err := netip.ParsePrefix(prefix); err != nil {
			return fmt.Errorf("invalid route prefix")
		}
		for i, row := range routes[prefix] {
			ref := fmt.Sprintf("json:/%s/%d", pointerPart(prefix), i)
			r := driver.Route{Prefix: prefix, Protocol: row.Protocol, Context: row.VRF, Selected: row.Selected, Installed: row.Installed, NextHops: []driver.NextHop{}, Evidence: ref}
			for j, nh := range row.NextHops {
				if nh.IP != "" {
					if _, err := netip.ParseAddr(nh.IP); err != nil {
						return fmt.Errorf("invalid next-hop address")
					}
				}
				r.NextHops = append(r.NextHops, driver.NextHop{Address: nh.IP, Interface: nh.Interface, Active: nh.Active, Installed: nh.FIB, Evidence: fmt.Sprintf("%s/nexthops/%d", ref, j)})
			}
			f.Routes = append(f.Routes, r)
		}
	}
	f.Notes = append(f.Notes, "Route state is a later lookup in the reported context; it does not establish the earlier probe's forwarding path.")
	return nil
}

type value struct {
	Value string `json:"value"`
}

func oneValue(v []value) string {
	if len(v) == 1 {
		return v[0].Value
	}
	return ""
}
func parseLLDP(data []byte, f *driver.Facts) error {
	var doc struct {
		LLDP *[]struct {
			Interfaces []struct {
				Name    string `json:"name"`
				Chassis []struct {
					Name, ID   []value
					Management []value `json:"mgmt-ip"`
				} `json:"chassis"`
				Ports []struct {
					ID []value `json:"id"`
				} `json:"port"`
			} `json:"interface"`
		} `json:"lldp"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.LLDP == nil {
		return fmt.Errorf("expected LLDP document")
	}
	for i, group := range *doc.LLDP {
		for j, item := range group.Interfaces {
			if item.Name == "" {
				return fmt.Errorf("LLDP entry lacks local interface")
			}
			ref := fmt.Sprintf("json:/lldp/%d/interface/%d", i, j)
			n := driver.Neighbor{Interface: item.Name, ManagementAddresses: []string{}, Evidence: ref}
			if len(item.Chassis) == 1 {
				chassis := item.Chassis[0]
				n.Name, n.ChassisID = oneValue(chassis.Name), oneValue(chassis.ID)
				for _, address := range chassis.Management {
					a, err := netip.ParseAddr(address.Value)
					if err != nil {
						return fmt.Errorf("invalid LLDP management address")
					}
					n.ManagementAddresses = append(n.ManagementAddresses, a.Unmap().String())
				}
			}
			if len(item.Ports) == 1 {
				n.PortID = oneValue(item.Ports[0].ID)
			}
			f.Neighbors = append(f.Neighbors, n)
		}
	}
	f.Notes = append(f.Notes, "LLDP identifies reported adjacency, not proof of an earlier packet's path.")
	return nil
}
func parseVersion(data []byte, f *driver.Facts) error {
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(value, "\"") {
			v, err := strconv.Unquote(value)
			if err != nil {
				return err
			}
			value = v
		}
		fields[key] = value
	}
	if fields["ID"] != "cumulus-linux" {
		return fmt.Errorf("OS identity is not Cumulus Linux")
	}
	f.Version = &driver.Version{OS: fields["NAME"], Version: fields["VERSION_ID"], Evidence: "file"}
	return nil
}
func numbers(data []byte, count int) ([]float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) < count {
		return nil, fmt.Errorf("missing numeric fields")
	}
	out := make([]float64, count)
	for i := range out {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid numeric field")
		}
		out[i] = v
	}
	return out, nil
}
