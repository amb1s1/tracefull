package inventory

import (
	"strings"
	"testing"
)

const example = `defaults:
  ssh_user: readonly
devices:
  - name: leaf
    address: 192.0.2.10
    platform: cumulus
    addresses: [192.0.2.1/31]
  - name: leaf-long
    address: leaf-long.example
    platform: new-os
    addresses: [192.0.2.1]
`

func TestMatch(t *testing.T) {
	inv, w, e := Load([]byte(example))
	if e != nil || len(w) != 2 {
		t.Fatalf("%v %v", e, w)
	}
	tests := []struct {
		ip           string
		names        []string
		over         map[string]string
		want, method string
	}{
		{"192.0.2.1", nil, nil, "", "addresses-ambiguous"}, {"192.0.2.0", nil, nil, "", "unmatched"}, {"192.0.2.10", nil, nil, "leaf", "address"}, {"192.0.2.20", []string{"leaf."}, nil, "leaf", "rdns-exact"}, {"192.0.2.20", []string{"leaf-long-et1.example."}, nil, "leaf-long", "rdns-prefix"}, {"192.0.2.1", nil, map[string]string{"192.0.2.1": "leaf"}, "leaf", "map"},
	}
	for _, tc := range tests {
		m := inv.Match(tc.ip, tc.names, tc.over)
		if m.Device != tc.want || m.Method != tc.method {
			t.Fatalf("%+v: %+v", tc, m)
		}
	}
	if inv.ValidateOverrides(map[string]string{"bad": "leaf"}) == nil || inv.ValidateOverrides(map[string]string{"192.0.2.1": "missing"}) == nil {
		t.Fatal("invalid override accepted")
	}
	if inv.ValidateOverrides(map[string]string{"192.0.2.1": "leaf"}) != nil {
		t.Fatal("valid map rejected")
	}
}
func TestInvalid(t *testing.T) {
	for _, s := range []string{"bad: true", "devices: [", "devices: [{}]", example + "---\ndevices: []", strings.Replace(example, "192.0.2.1/31", "bad", 1), strings.Replace(example, "name: leaf-long", "name: leaf", 1)} {
		if _, _, e := Load([]byte(s)); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestJSON(t *testing.T) {
	if _, _, e := Load([]byte(`{"devices":[{"name":"v6","address":"2001:db8::1","platform":"linux"}]}`)); e != nil {
		t.Fatal(e)
	}
}
