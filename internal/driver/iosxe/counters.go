package iosxe

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

var counterHeader = regexp.MustCompile(`^(\S+) is .+, line protocol is (up|down|administratively down)(?:\s|$)`)
var mtuRE = regexp.MustCompile(`MTU (\S+) bytes,`)
var speedRE = regexp.MustCompile(`(?:Full|Half)-duplex, ([+-]?[0-9.]+)([MG])b/s,`)
var inputErrorRE = regexp.MustCompile(`(\S+) input errors`)
var outputErrorRE = regexp.MustCompile(`(\S+) output errors`)
var inputDropsRE = regexp.MustCompile(`Input queue: [^/\s]+/[^/\s]+/([^/\s]+)/[^/\s]+ \(size/max/drops/flushes\)`)
var outputDropsRE = regexp.MustCompile(`Total output drops: (\S+)`)

func counters(lines []string, f *driver.Facts) error {
	c := driver.Counter{}
	for i, l := range lines {
		if m := counterHeader.FindStringSubmatch(l); m != nil {
			if c.Interface != "" || !driver.InterfaceName(m[1]) {
				return fmt.Errorf("invalid/multiple counter interfaces")
			}
			c.Interface = interfaceName(m[1])
			c.State = m[2]
			c.Evidence = ref(i)
		}
		for _, x := range []struct {
			re *regexp.Regexp
			p  **uint64
		}{{mtuRE, &c.MTUBytes}, {inputErrorRE, &c.RXErrors}, {outputErrorRE, &c.TXErrors}, {inputDropsRE, &c.RXDrops}, {outputDropsRE, &c.TXDrops}} {
			if m := x.re.FindStringSubmatch(l); m != nil {
				if *x.p != nil {
					return fmt.Errorf("duplicate counter measurement")
				}
				v, e := strconv.ParseUint(m[1], 10, 64)
				if e != nil {
					return e
				}
				*x.p = &v
			}
		}
		if m := speedRE.FindStringSubmatch(l); m != nil {
			if c.SpeedBPS != nil {
				return fmt.Errorf("duplicate line speed")
			}
			v, e := strconv.ParseUint(m[1], 10, 64)
			factor := uint64(1000000)
			if m[2] == "G" {
				factor *= 1000
			}
			if e != nil || v > ^uint64(0)/factor {
				return fmt.Errorf("speed overflow")
			}
			v *= factor
			c.SpeedBPS = &v
		}
	}
	if c.Interface == "" {
		return fmt.Errorf("missing interface counters")
	}
	f.Counters = append(f.Counters, c)
	f.Notes = append(f.Notes, "Counters are cumulative. Input queue drops and total output drops retain their reported scope; configurable BW is not line speed. MTU framing is unknown.")
	return nil
}
func optics(iface string, lines []string, f *driver.Facts) error {
	section := ""
	o := driver.Optical{Interface: iface}
	seen := map[string]bool{}
	for i, l := range lines {
		if strings.Contains(l, "Temperature") && !strings.Contains(l, "Port") {
			section = "temperature"
		}
		if strings.Contains(l, "Voltage") {
			section = "voltage"
		}
		if strings.Contains(l, "Current") {
			section = "current"
		}
		if strings.Contains(l, "Transmit Power") {
			section = "tx"
		}
		if strings.Contains(l, "Receive Power") {
			section = "rx"
		}
		fields := strings.Fields(l)
		if len(fields) == 0 || (!strings.HasPrefix(fields[0], "Te") || !strings.Contains(fields[0], "/")) {
			continue
		}
		if interfaceName(fields[0]) != iface {
			return fmt.Errorf("optics interface mismatch")
		}
		index := 1
		if section != "temperature" && section != "voltage" {
			index = 2
			if len(fields) < 3 || fields[1] != "N/A" {
				return fmt.Errorf("unvalidated optical lane layout")
			}
		}
		if len(fields) <= index {
			return fmt.Errorf("truncated DOM row")
		}
		if seen[section] {
			return fmt.Errorf("duplicate DOM measurement")
		}
		seen[section] = true
		if section != "temperature" && section != "tx" && section != "rx" {
			continue
		}
		if fields[index] == "N/A" || fields[index] == "NA" {
			continue
		}
		v, e := strconv.ParseFloat(fields[index], 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("invalid DOM value")
		}
		switch section {
		case "temperature":
			o.TemperatureCelsius = &v
		case "tx":
			o.TXPowerDBM = &v
		case "rx":
			o.RXPowerDBM = &v
		}
		if o.Evidence == "" {
			o.Evidence = ref(i)
		}
	}
	if !seen["temperature"] || !seen["tx"] || !seen["rx"] {
		return fmt.Errorf("incomplete/unvalidated transceiver layout")
	}
	if o.Evidence == "" {
		o.Evidence = "file"
	}
	f.Optics = append(f.Optics, o)
	f.Notes = append(f.Notes, "Captured DOM lane is N/A, retained as unknown. Values are reported temperature and RX/TX dBm, not optic-health findings.")
	return nil
}
