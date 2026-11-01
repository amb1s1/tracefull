package nxos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

type node struct {
	data json.RawMessage
	ref  string
}

// NX-OS represents a single ROW as an object and multiple ROWs as an array.
// References retain the actual shape rather than inventing array indexes.
func children(n node, table, row string, optional bool) ([]node, error) {
	var parent, mapTable map[string]json.RawMessage
	if err := json.Unmarshal(n.data, &parent); err != nil || parent == nil {
		return nil, fmt.Errorf("invalid object at %s", n.ref)
	}
	raw, ok := parent[table]
	if !ok && optional {
		return []node{}, nil
	}
	if err := json.Unmarshal(raw, &mapTable); err != nil || mapTable == nil {
		return nil, fmt.Errorf("missing table %s", table)
	}
	raw = bytes.TrimSpace(mapTable[row])
	path := n.ref + "/" + table + "/" + row
	if len(raw) == 0 {
		return nil, fmt.Errorf("missing row %s", row)
	}
	if raw[0] == '{' {
		return []node{{raw, path}}, nil
	}
	if raw[0] != '[' {
		return nil, fmt.Errorf("invalid rows at %s", path)
	}
	var all []json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	out := make([]node, 0, len(all))
	for i, r := range all {
		if len(bytes.TrimSpace(r)) == 0 || bytes.TrimSpace(r)[0] != '{' {
			return nil, fmt.Errorf("invalid row at %s/%d", path, i)
		}
		out = append(out, node{r, fmt.Sprintf("%s/%d", path, i)})
	}
	return out, nil
}
func uintValue(s string) (*uint64, error) {
	if s == "" {
		return nil, nil
	}
	n, e := strconv.ParseUint(s, 10, 64)
	return &n, e
}
func floatValue(s string) (*float64, error) {
	if s == "" || s == "N/A" {
		return nil, nil
	}
	n, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, fmt.Errorf("invalid numeric measurement %q", s)
	}
	return &n, nil
}
func boolValue(s string) (*bool, error) {
	if s == "" {
		return nil, nil
	}
	if s != "true" && s != "false" {
		return nil, fmt.Errorf("invalid boolean %q", s)
	}
	v := s == "true"
	return &v, nil
}
