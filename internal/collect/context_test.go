package collect

import (
	"reflect"
	"testing"

	"github.com/amb1s1/tracefull/internal/driver"
)

func TestEgressDiscoveryRequiresRequestedContext(t *testing.T) {
	// Synthetic mixed/unknown-context results must not select unrelated egress.
	yes := true
	facts := driver.Empty()
	for _, context := range []string{"default", "mgmt", ""} {
		facts.Routes = append(facts.Routes, driver.Route{
			Context: context, Selected: &yes, Installed: &yes,
			NextHops: []driver.NextHop{{Interface: "port-" + context, Active: &yes, Installed: &yes}},
		})
	}
	// Ownership observations remain useful even when the route context differs.
	facts.Addresses = []driver.Address{{Address: "192.0.2.1", Interface: "ingress"}}
	commands := []CommandResult{{Facts: &facts}}
	for _, context := range []string{"default", "mgmt"} {
		got := interfaces([]string{"192.0.2.1"}, commands, context)
		want := []string{"ingress", "port-" + context}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("context %s: got %v, want %v", context, got, want)
		}
	}
}
