package parseutil

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// UnixTimestamp preserves the source's fractional seconds without float rounding.
func UnixTimestamp(raw json.RawMessage) (*string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	s := strings.Trim(string(raw), "\"")
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid Unix timestamp")
	}
	sec, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || sec < 0 || sec > 253402300799 {
		return nil, fmt.Errorf("invalid Unix seconds")
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > 9 {
		return nil, fmt.Errorf("timestamp exceeds nanosecond precision")
	}
	for _, c := range frac {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("invalid timestamp fraction")
		}
	}
	ns, e := strconv.ParseInt(frac+strings.Repeat("0", 9-len(frac)), 10, 64)
	if e != nil {
		return nil, e
	}
	out := time.Unix(sec, ns).UTC().Format(time.RFC3339Nano)
	return &out, nil
}
func Earlier(a, b *string) *string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	at, _ := time.Parse(time.RFC3339Nano, *a)
	bt, _ := time.Parse(time.RFC3339Nano, *b)
	if bt.Before(at) {
		return b
	}
	return a
}
func Later(a, b *string) *string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	at, _ := time.Parse(time.RFC3339Nano, *a)
	bt, _ := time.Parse(time.RFC3339Nano, *b)
	if bt.After(at) {
		return b
	}
	return a
}
