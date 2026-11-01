package routeros

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

func fields(b []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, ";;;") {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		if _, exists := out[k]; exists {
			return nil, fmt.Errorf("duplicate text field %q", k)
		}
		out[k] = strings.TrimSpace(v)
	}
	return out, nil
}

var durationRE = regexp.MustCompile(`^(?:[0-9]+w)?(?:[0-9]+d)?(?:[0-9]+h)?(?:[0-9]+m)?(?:[0-9]+s)?$`)
var unitRE = regexp.MustCompile(`([0-9]+)([wdhms])`)

func resource(name string, b []byte, f *driver.Facts) error {
	v, e := fields(b)
	if e != nil {
		return e
	}
	if v["platform"] != "MikroTik" || v["version"] == "" {
		return fmt.Errorf("missing RouterOS identity")
	}
	if name == "version" {
		f.Version = &driver.Version{OS: "MikroTik RouterOS", Version: v["version"], Evidence: "file"}
		if s := v["uptime"]; s != "" {
			if !durationRE.MatchString(s) {
				return fmt.Errorf("unsupported uptime format")
			}
			var sum uint64
			for _, m := range unitRE.FindAllStringSubmatch(s, -1) {
				x, e := strconv.ParseUint(m[1], 10, 64)
				factor := map[string]uint64{"w": 604800, "d": 86400, "h": 3600, "m": 60, "s": 1}[m[2]]
				if e != nil || x > (^uint64(0)-sum)/factor {
					return fmt.Errorf("uptime overflow")
				}
				sum += x * factor
			}
			seconds := float64(sum)
			f.UptimeSeconds = &seconds
			f.UptimeEvidence = "file"
		}
	} else {
		raw, ok := strings.CutSuffix(v["cpu-load"], "%")
		if !ok {
			return fmt.Errorf("missing CPU percent")
		}
		n, e := strconv.ParseFloat(raw, 64)
		if e != nil || math.IsNaN(n) || n < 0 || n > 100 {
			return fmt.Errorf("invalid CPU percentage")
		}
		f.CPU = &driver.CPU{Utilization: &n, Evidence: "file"}
		f.Notes = append(f.Notes, "CPU utilization is the resource command's reported percentage with an unspecified sampling interval; load averages are unknown.")
	}
	return nil
}
func optics(iface string, b []byte, f *driver.Facts) error {
	v, e := fields(b)
	if e != nil {
		return e
	}
	if v["name"] != iface || v["status"] == "" {
		return fmt.Errorf("missing/mismatched Ethernet monitor identity")
	}
	if v["sfp-module-present"] != "yes" && v["sfp-module-present"] != "no" {
		return fmt.Errorf("unsupported module-presence format")
	}
	o := driver.Optical{Interface: iface, Evidence: "file"}
	for _, x := range []struct {
		key, unit string
		p         **float64
	}{{"sfp-temperature", "C", &o.TemperatureCelsius}, {"sfp-tx-power", "dBm", &o.TXPowerDBM}, {"sfp-rx-power", "dBm", &o.RXPowerDBM}} {
		s := v[x.key]
		if s == "" || s == "N/A" {
			continue
		}
		raw, ok := strings.CutSuffix(s, x.unit)
		if !ok {
			return fmt.Errorf("unvalidated optical unit")
		}
		n, e := strconv.ParseFloat(raw, 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("invalid optical reading")
		}
		*x.p = &n
	}
	if v["sfp-module-present"] == "yes" {
		f.Optics = append(f.Optics, o)
	} else if o.TemperatureCelsius != nil || o.RXPowerDBM != nil || o.TXPowerDBM != nil {
		return fmt.Errorf("absent module with readings")
	}
	f.Notes = append(f.Notes, "Single-module monitor values retain explicit Celsius/dBm units and unknown lane identity. Unvalidated per-lane layouts and optic-health diagnosis are unsupported.")
	return nil
}
