package junos

import (
	"encoding/json"
	"fmt"
)

func routeActive(entry node) (*bool, error) {
	rows := entry.list("active-tag")
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("multiple active route tags")
	}
	var tag string
	if err := json.Unmarshal(rows[0]["data"], &tag); err != nil {
		return nil, fmt.Errorf("invalid active route tag")
	}
	switch tag {
	case "":
		return nil, nil
	case "*", "+", "-":
		active := tag != "-"
		return &active, nil
	default:
		return nil, fmt.Errorf("unknown active route tag %q", tag)
	}
}

func nextHopActive(nh node) (*bool, error) {
	rows := nh.list("selected-next-hop")
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("multiple selected-next-hop markers")
	}
	// The captured XML presence marker is data:[null]. A string such as "false"
	// is malformed evidence, not proof that the next hop is selected.
	var flag []json.RawMessage
	if err := json.Unmarshal(rows[0]["data"], &flag); err != nil || len(flag) != 1 || string(flag[0]) != "null" {
		return nil, fmt.Errorf("invalid selected-next-hop marker")
	}
	selected := true
	return &selected, nil
}
