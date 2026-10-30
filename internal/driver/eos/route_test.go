package eos

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestEmptyRouteCapture(t *testing.T) {
	f, err := (Driver{}).Parse("route", fixture(t, "route4-empty"))
	if err != nil || f.Routes == nil || len(f.Routes) != 0 {
		t.Fatal("empty lookup became a route", f, err)
	}
}

func TestMalformedRouteContainers(t *testing.T) {
	// Synthetic structural failures, not additional device captures.
	for _, data := range []string{
		`{"vrfs":{"default":null}}`,
		`{"vrfs":{"default":{}}}`,
		`{"vrfs":{"default":{"routes":null}}}`,
		`{"vrfs":{"default":{"routes":{"::/0":null}}}}`,
		`{"vrfs":{"default":{"routes":{"::/0":{"routeAction":"forward","hardwareProgrammed":true,"vias":[null]}}}}}`,
		`{"vrfs":{"default":{"routes":{"::/0":{"routeAction":"forward","hardwareProgrammed":true,"vias":[{}]}}}}}`,
	} {
		t.Run(data, func(t *testing.T) {
			f, err := (Driver{}).Parse("route", []byte(data))
			if err == nil || !reflect.DeepEqual(f, driver.Empty()) {
				t.Fatal("malformed structure produced successful facts", f, err)
			}
		})
	}
}

func TestRouteUnknownProgramming(t *testing.T) {
	// Synthetic missing optional fields must not inherit VRF-wide programming state.
	var doc map[string]any
	if err := json.Unmarshal(fixture(t, "route6"), &doc); err != nil {
		t.Fatal(err)
	}
	vrf := doc["vrfs"].(map[string]any)["default"].(map[string]any)
	row := vrf["routes"].(map[string]any)["::/0"].(map[string]any)
	delete(row, "hardwareProgrammed")
	delete(row, "routeAction")
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	f, err := (Driver{}).Parse("route", data)
	if err != nil || len(f.Routes) != 1 {
		t.Fatal(f, err)
	}
	r := f.Routes[0]
	if r.Selected != nil || r.Installed != nil || r.NextHops[0].Active != nil || r.NextHops[0].Installed != nil {
		t.Fatal("missing route state became known", r)
	}
}
