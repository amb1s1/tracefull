package iosxr

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

var isisCaptures = []string{"route4-isis-single", "route4-isis-multipath", "route6-isis-multipath"}

func TestISISCaptures(t *testing.T) {
	data, err := os.ReadFile("testdata/route-isis.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string][]driver.Route
	if err = json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	for _, name := range isisCaptures {
		t.Run(name, func(t *testing.T) {
			for _, input := range []string{string(fixture(t, name)), strings.ReplaceAll(string(fixture(t, name)), "\n", "\r\n")} {
				f, err := (Driver{}).Parse("route", []byte(input))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(f.Routes, golden[name]) {
					t.Fatalf("routes: %+v; want %+v", f.Routes, golden[name])
				}
				if len(f.Notes) == 0 {
					t.Fatal("missing interpretation limits")
				}
			}
		})
	}
}

func TestISISFailures(t *testing.T) {
	original := string(fixture(t, "route6-isis-multipath"))
	// Synthetic mutations test failures; they are not new platform fixtures.
	cases := map[string]string{
		"contradictory absence first": "% Network not in table\n" + original,
		"contradictory absence last":  original + "% Network not in table\n",
		"duplicate installation":      strings.Replace(original, "  Installed", "  Installed Oct 29\n  Installed", 1),
		"unsupported protocol":        strings.Replace(original, `"isis 1"`, `"bgp 64512"`, 1),
		"duplicate protocol":          strings.Replace(original, "  Installed", "  Known via \"isis 1\"\n  Installed", 1),
		"malformed next hop":          strings.Replace(original, "fe80::1, from", "invalid, from", 1),
		"wrong family":                strings.Replace(original, "fe80::1, from", "192.0.2.1, from", 1),
		"scoped address":              strings.Replace(original, "fe80::1, from", "fe80::1%eth0, from", 1),
		"unspecified address":         strings.Replace(original, "fe80::1, from", "::, from", 1),
		"multicast address":           strings.Replace(original, "fe80::1, from", "ff02::1, from", 1),
		"mapped address":              strings.Replace(original, "fe80::1, from", "::ffff:192.0.2.1, from", 1),
		"invalid originator":          strings.Replace(original, "from 2001:db8:30::33", "from bad", 1),
		"bad interface":               strings.Replace(original, "via FourHundredGigE0/0/0/4", "via -bad", 1),
		"missing interface":           strings.Replace(original, ", via FourHundredGigE0/0/0/4", "", 1),
		"recursive":                   strings.Replace(original, "fe80::1, from", "fe80::1, recursive, from", 1),
		"backup":                      strings.Replace(original, "      Label: None", "      Backup path", 1),
		"tunnel":                      strings.Replace(original, "Tunnel ID: None", "Tunnel ID: 17", 1),
		"extended attributes":         strings.Replace(original, "Extended communities count: 0", "Extended communities count: 1", 1),
		"metric absent":               strings.Replace(original, "      Route metric is 105\n", "", 1),
		"metric duplicate":            strings.Replace(original, "      Route metric is 105\n", "      Route metric is 105\n      Route metric is 105\n", 1),
		"last metric absent":          strings.ReplaceAll(original, "      Route metric is 105\n", ""),
		"header absent":               strings.Replace(original, "  Routing Descriptor Blocks\n", "", 1),
		"header duplicate":            strings.Replace(original, "  Routing Descriptor Blocks\n", "  Routing Descriptor Blocks\n  Routing Descriptor Blocks\n", 1),
		"empty descriptors":           "Routing entry for 2001:db8::/64\n  Known via \"isis 1\"\n  Routing Descriptor Blocks\n  Route version is 0x1 (1)\n",
		"truncated":                   strings.Split(original, "  Route version is")[0],
		"bad indentation":             strings.Replace(original, "    fe80::1, from", "   fe80::1, from", 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			f, err := (Driver{}).Parse("route", []byte(data))
			if err == nil || len(f.Routes) != 0 {
				t.Fatalf("accepted invalid/partial route: %v %+v", err, f)
			}
		})
	}
}

func TestISISUnknownSelection(t *testing.T) {
	lines := strings.Split(string(fixture(t, "route4-isis-single")), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "  Installed ") {
			lines[i] = ""
		}
	}
	f, err := (Driver{}).Parse("route", []byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	r := f.Routes[0]
	if r.Selected != nil || r.Installed != nil || r.NextHops[0].Active != nil || r.NextHops[0].Installed != nil {
		t.Fatal("invented selection or hardware state", r)
	}
}
