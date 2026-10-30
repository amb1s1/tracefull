package cumulus

import "testing"

func TestPlainAddressesDoNotInventVRFDeviceContext(t *testing.T) {
	f, err := (Driver{}).Parse("addresses", capture(t, "addresses.json"))
	if err != nil {
		t.Fatal(err)
	}
	vrfAddresses := 0
	for _, a := range f.Addresses {
		if a.Interface == "mgmt" || a.Interface == "tenant-a" {
			vrfAddresses++
			if a.RoutingContext != nil {
				t.Errorf("plain capture cannot establish context of master interface %s: %s", a.Interface, *a.RoutingContext)
			}
		}
		if a.Interface == "lo" && (a.RoutingContext == nil || *a.RoutingContext != "default") {
			t.Error("lost standalone loopback context")
		}
	}
	if vrfAddresses == 0 {
		t.Fatal("fixture lacks VRF-device addresses")
	}
}

func TestMissingInterfaceFlagsKeepContextUnknown(t *testing.T) {
	// Synthetic incomplete address record: no evidence excludes a VRF device.
	f, err := (Driver{}).Parse("addresses", []byte(`[{"ifname":"example","addr_info":[{"local":"192.0.2.1"}]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Addresses[0].RoutingContext != nil {
		t.Fatal("missing interface metadata became default context")
	}
}
