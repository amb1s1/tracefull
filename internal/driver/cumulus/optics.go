package cumulus

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/amb1s1/tracefull/internal/driver"
)

// Only the captured CMIS OSFP labels are accepted. Other module layouts need
// their own real captures before they can be interpreted.
var opticalChannel = regexp.MustCompile(`^(Transmit avg optical power|Rcvr signal avg optical power) \(Channel ([1-8])\)$`)
var opticalPower = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?) mW / (-?[0-9]+(?:\.[0-9]+)?|-inf) dBm$`)
var opticalTemperature = regexp.MustCompile(`^(-?[0-9]+(?:\.[0-9]+)?) degrees C / -?[0-9]+(?:\.[0-9]+)? degrees F$`)

func parseOptics(iface string, data []byte, f *driver.Facts) error {
	if !ifaceName.MatchString(iface) {
		return fmt.Errorf("invalid optics interface")
	}
	fields, err := opticalFields(data)
	if err != nil {
		return err
	}
	if fields["Identifier"] != "0x19 (OSFP 8X Pluggable Transceiver)" || fields["Revision compliance"] != "Rev. 4.0" {
		return fmt.Errorf("unvalidated transceiver layout; expected captured OSFP revision 4.0 format")
	}
	var temperature *float64
	if raw, ok := fields["Module temperature"]; ok {
		m := opticalTemperature.FindStringSubmatch(raw)
		if m == nil {
			return fmt.Errorf("invalid module temperature")
		}
		temperature, err = finiteOptical(m[1])
		if err != nil {
			return err
		}
		if *temperature < -273.15 {
			return fmt.Errorf("module temperature below absolute zero")
		}
	}
	f.Optics, err = opticalLanes(iface, fields, temperature)
	if err != nil {
		return err
	}
	f.Notes = append(f.Notes, "OSFP module temperature is shared across reported lanes. Missing or nonfinite optical power remains unknown; alarm flags and thresholds are retained only in raw evidence. These readings alone do not establish optic health.")
	if len(f.Optics) == 0 {
		f.Notes = append(f.Notes, "No supported optical measurements were present in the module output.")
	}
	return nil
}

func opticalFields(data []byte) (map[string]string, error) {
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		key = strings.TrimSpace(key)
		current := key == "Identifier" || key == "Revision compliance" || key == "Module temperature" || strings.HasPrefix(key, "Transmit avg optical power") || strings.HasPrefix(key, "Rcvr signal avg optical power")
		if !current {
			continue
		}
		if !ok {
			return nil, fmt.Errorf("truncated optical field")
		}
		if strings.Contains(key, "optical power") && opticalChannel.FindStringSubmatch(key) == nil {
			return nil, fmt.Errorf("unvalidated optical channel label")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate optical field")
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields, nil
}

func parseOpticalPower(raw string) (*float64, error) {
	m := opticalPower.FindStringSubmatch(raw)
	if m == nil {
		return nil, fmt.Errorf("invalid optical power")
	}
	mw, err := finiteOptical(m[1])
	if err != nil {
		return nil, err
	}
	if m[2] == "-inf" {
		if *mw != 0 {
			return nil, fmt.Errorf("nonzero power with negative infinity dBm")
		}
		return nil, nil
	}
	return finiteOptical(m[2])
}
func finiteOptical(raw string) (*float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("invalid optical number")
	}
	return &v, nil
}

func opticalLanes(iface string, fields map[string]string, temperature *float64) ([]driver.Optical, error) {
	out := []driver.Optical{}
	lanes := map[string]driver.Optical{}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		m := opticalChannel.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		power, err := parseOpticalPower(fields[key])
		if err != nil {
			return nil, err
		}
		row := lanes[m[2]]
		row.Interface = iface
		row.Lane = m[2]
		row.TemperatureCelsius = temperature
		row.Evidence = "file"
		if m[1] == "Transmit avg optical power" {
			row.TXPowerDBM = power
		} else {
			row.RXPowerDBM = power
		}
		lanes[m[2]] = row
	}
	for lane := 1; lane <= 8; lane++ {
		if row, ok := lanes[strconv.Itoa(lane)]; ok {
			out = append(out, row)
		}
	}
	if len(lanes) == 0 && temperature != nil {
		out = append(out, driver.Optical{Interface: iface, TemperatureCelsius: temperature, Evidence: "file"})
	}
	return out, nil
}
