package parseutil

import (
	"encoding/json"
	"testing"
)

func TestTime(t *testing.T) {
	a, e := UnixTimestamp(json.RawMessage(`"1565903869.420662"`))
	if e != nil || *a != "2019-08-15T21:17:49.420662Z" {
		t.Fatalf("%v %v", a, e)
	}
	b, e := UnixTimestamp(json.RawMessage(`1565903870`))
	if e != nil {
		t.Fatal(e)
	}
	if Earlier(a, b) != a || Later(a, b) != b || Earlier(nil, b) != b || Later(a, nil) != a || Earlier(b, a) != a || Later(b, a) != b {
		t.Fatal("window ordering")
	}
	for _, s := range []string{`"bad"`, `"1.2.3"`, `"1.abc"`, `"-1.0"`, `"1.1234567890"`} {
		if _, e := UnixTimestamp(json.RawMessage(s)); e == nil {
			t.Fatal(s)
		}
	}
}
