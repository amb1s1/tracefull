package inventory

import (
	"bytes"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/amb1s1/tracefull/internal/trace"
)

type Device struct {
	Name      string   `yaml:"name" json:"name"`
	Address   string   `yaml:"address" json:"address"`
	Platform  string   `yaml:"platform" json:"platform"`
	Role      string   `yaml:"role" json:"role"`
	Site      string   `yaml:"site" json:"site"`
	Notes     string   `yaml:"notes" json:"notes"`
	Addresses []string `yaml:"addresses" json:"addresses"`
}
type Inventory struct {
	Defaults struct {
		SSHUser  string `yaml:"ssh_user" json:"ssh_user"`
		JumpHost string `yaml:"jump_host" json:"jump_host"`
	} `yaml:"defaults" json:"defaults"`
	Devices []Device `yaml:"devices" json:"devices"`
}

func address(s string) string {
	if a, e := netip.ParseAddr(s); e == nil {
		return a.Unmap().String()
	}
	if p, e := netip.ParsePrefix(s); e == nil {
		return p.Addr().Unmap().String()
	}
	return ""
}
func Load(b []byte) (Inventory, []string, error) {
	var inv Inventory
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e := d.Decode(&inv); e != nil {
		return inv, nil, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return inv, nil, fmt.Errorf("inventory must contain exactly one document")
	}
	if inv.Devices == nil {
		return inv, nil, fmt.Errorf("inventory requires a devices array")
	}
	warnings := []string{}
	names := map[string]bool{}
	ips := map[string]string{}
	for i := range inv.Devices {
		v := &inv.Devices[i]
		if v.Name == "" || v.Address == "" || v.Platform == "" {
			return inv, nil, fmt.Errorf("device requires name, address and platform")
		}
		if names[v.Name] {
			return inv, nil, fmt.Errorf("duplicate device name %q", v.Name)
		}
		names[v.Name] = true
		if !known(v.Platform) {
			warnings = append(warnings, "Unknown platform on "+v.Name+": "+v.Platform)
		}
		for _, raw := range append([]string{v.Address}, v.Addresses...) {
			ip := address(raw)
			if ip == "" {
				if raw != v.Address {
					return inv, nil, fmt.Errorf("invalid interface address %q", raw)
				}
				continue
			}
			if owner, ok := ips[ip]; ok && owner != v.Name {
				warnings = append(warnings, "Duplicate address "+ip+": "+owner+", "+v.Name)
			} else {
				ips[ip] = v.Name
			}
		}
	}
	sort.Strings(warnings)
	return inv, warnings, nil
}
func known(s string) bool {
	switch s {
	case "cisco-ios-xr", "iosxr", "cisco-ios-xe", "cisco-nxos", "arista-eos", "junos", "nvidia-cumulus", "frr", "linux", "mikrotik-routeros", "ios", "nxos", "eos", "cumulus", "routeros":
		return true
	}
	return false
}
func (inv Inventory) ValidateOverrides(overrides map[string]string) error {
	for ip, name := range overrides {
		if _, e := netip.ParseAddr(ip); e != nil {
			return fmt.Errorf("invalid map address %q", ip)
		}
		found := false
		for _, d := range inv.Devices {
			found = found || d.Name == name
		}
		if !found {
			return fmt.Errorf("map references unknown device %q", name)
		}
	}
	return nil
}
func (inv Inventory) Match(ip string, names []string, overrides map[string]string) trace.Match {
	if name, ok := overrides[ip]; ok {
		return trace.Match{Device: name, Method: "map", Candidates: []string{name}}
	}
	for _, method := range []string{"addresses", "address", "rdns-exact", "rdns-prefix"} {
		candidates := []string{}
		longest := 0
		for _, d := range inv.Devices {
			match := false
			switch method {
			case "addresses":
				for _, a := range d.Addresses {
					match = match || address(a) == ip
				}
			case "address":
				match = address(d.Address) == ip
			case "rdns-exact":
				for _, n := range names {
					match = match || strings.EqualFold(strings.TrimSuffix(n, "."), d.Name)
				}
			case "rdns-prefix":
				for _, n := range names {
					label := strings.Split(strings.ToLower(n), ".")[0]
					dn := strings.ToLower(d.Name)
					if strings.HasPrefix(label, dn+"-") || strings.HasPrefix(label, dn+"_") {
						match = true
					}
				}
			}
			if match {
				if method == "rdns-prefix" {
					if len(d.Name) < longest {
						continue
					}
					if len(d.Name) > longest {
						candidates = nil
						longest = len(d.Name)
					}
				}
				candidates = append(candidates, d.Name)
			}
		}
		if len(candidates) > 0 {
			sort.Strings(candidates)
			m := trace.Match{Method: method, Candidates: candidates}
			if len(candidates) == 1 {
				m.Device = candidates[0]
			} else {
				m.Method += "-ambiguous"
			}
			return m
		}
	}
	return trace.Match{Method: "unmatched", Candidates: []string{}}
}
