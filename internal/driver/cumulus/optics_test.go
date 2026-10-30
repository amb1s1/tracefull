package cumulus

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestOSFPCapture(t *testing.T) {
	f, err := (Driver{}).Parse("optics-swp49", capture(t, "optics-osfp.txt"))
	if err != nil || len(f.Optics) != 8 {
		t.Fatal(f, err)
	}
	var want []driver.Optical
	if err := json.Unmarshal(capture(t, "optics.golden.json"), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Optics, want) {
		t.Fatalf("optics golden differs: %+v", f.Optics)
	}
	if _, err := json.Marshal(f); err != nil {
		t.Fatal(err)
	}
}

func TestOpticsMissingAndNonfinite(t *testing.T) {
	// Synthetic subsets/mutations retain captured headers and labels, not new platform evidence.
	raw := string(capture(t, "optics-osfp.txt"))
	for _, tc := range []struct{ name, remove string }{
		{"temperature", "Module temperature                        :"},
		{"rx", "Rcvr signal avg optical power (Channel 1)"},
		{"tx", "Transmit avg optical power (Channel 1)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{}
			for _, line := range strings.Split(raw, "\n") {
				if !strings.Contains(line, tc.remove) {
					lines = append(lines, line)
				}
			}
			f, err := (Driver{}).Parse("optics-swp49", []byte(strings.Join(lines, "\n")))
			if err != nil || len(f.Optics) != 8 {
				t.Fatal(f, err)
			}
			o := f.Optics[0]
			if tc.name == "temperature" && o.TemperatureCelsius != nil || tc.name == "rx" && o.RXPowerDBM != nil || tc.name == "tx" && o.TXPowerDBM != nil {
				t.Fatal(o)
			}
		})
	}
	raw = strings.Replace(raw, "1.3640 mW / 1.35 dBm", "0.0000 mW / -inf dBm", 1)
	f, err := (Driver{}).Parse("optics-swp49", []byte(raw))
	if err != nil || f.Optics[0].TXPowerDBM != nil || f.Optics[0].RXPowerDBM == nil {
		t.Fatal(f, err)
	}
	header := "Identifier : 0x19 (OSFP 8X Pluggable Transceiver)\nRevision compliance : Rev. 4.0\n"
	for _, extra := range []string{"", "Module temperature : -1.00 degrees C / 30.20 degrees F\n"} {
		f, err = (Driver{}).Parse("optics-swp49", []byte(header+extra))
		if err != nil {
			t.Fatal(err)
		}
		if extra == "" && len(f.Optics) != 0 {
			t.Fatal("invented lanes", f)
		}
		if extra != "" && (len(f.Optics) != 1 || f.Optics[0].Lane != "" || *f.Optics[0].TemperatureCelsius != -1) {
			t.Fatal(f)
		}
	}
}

func TestOpticsRejectsMalformedAndUnvalidated(t *testing.T) {
	raw := string(capture(t, "optics-osfp.txt"))
	for _, tc := range []struct{ old, new string }{
		{"0x19 (OSFP 8X Pluggable Transceiver)", "0x18 (QSFP-DD)"},
		{"Rev. 4.0", "Rev. 5.0"},
		{"53.96 degrees C", "NaN degrees C"},
		{"53.96 degrees C", "-274.0 degrees C"},
		{"1.3640 mW / 1.35 dBm", "bad mW / 1.35 dBm"},
		{"1.3640 mW / 1.35 dBm", "1.3640 mW / -inf dBm"},
		{"1.3640 mW / 1.35 dBm", "1.3640 mW / +inf dBm"},
		{"1.3640 mW / 1.35 dBm", "1.3640 mW / 1.35 W"},
		{"Transmit avg optical power (Channel 1)", "Transmit avg optical power (Channel 9)"},
		{"1.3640 mW / 1.35 dBm", strings.Repeat("9", 400) + " mW / 1.35 dBm"},
	} {
		if f, err := (Driver{}).Parse("optics-swp49", []byte(strings.Replace(raw, tc.old, tc.new, 1))); err == nil || len(f.Optics) != 0 {
			t.Fatalf("accepted %q: %+v %v", tc.new, f, err)
		}
	}
	for _, suffix := range []string{"\nModule temperature : 1.0 degrees C / 33.8 degrees F\n", "\nTransmit avg optical power (Channel 1) : 1.0 mW / 0.0 dBm\n", "\nRcvr signal avg optical power (Channel 1)"} {
		if _, err := (Driver{}).Parse("optics-swp49", []byte(raw+suffix)); err == nil {
			t.Fatal("accepted duplicate/truncated field")
		}
	}
	if _, err := (Driver{}).Parse("optics-swp49;id", []byte(raw)); err == nil {
		t.Fatal("invalid interface")
	}
}

func TestOpticsCommandPolicy(t *testing.T) {
	for _, privileged := range []bool{false, true} {
		p := driver.Params{Destination: "192.0.2.1", Sudo: privileged, Interfaces: []string{"swp49", "eth0", "swp49"}}
		d := Driver{}
		plan, err := d.Plan([]string{"optics"}, p)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, c := range plan {
			if c.Group == "optics" {
				count++
				if c.Status != "planned" || d.Validate(c.Text, p) != nil {
					t.Fatal(c)
				}
			}
		}
		if count != 2 {
			t.Fatal(plan)
		}
		for _, cmd := range []string{"ethtool -m swp49 raw on", "ethtool -m swp49;id", "ethtool -m -h", "ethtool -E swp49", "ethtool -s swp49 speed 100", "ethtool --reset swp49 all", "ethtool -m swp49 | cat", "ethtool -m $(id)"} {
			if d.Validate(sudo(p)+cmd, p) == nil {
				t.Fatal("unsafe command", cmd)
			}
		}
		other := p
		other.Sudo = !p.Sudo
		if d.Validate(sudo(other)+"ethtool -m swp49", p) == nil {
			t.Fatal("privilege mismatch")
		}
	}
}
