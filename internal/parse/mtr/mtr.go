package mtr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string { return "mtr" }
func (Parser) Detect(b []byte) bool {
	return bytes.Contains(b, []byte(`"report"`)) || bytes.Contains(b, []byte("Loss%")) && bytes.Contains(b, []byte("Snt"))
}
func (Parser) Parse(b []byte) (trace.Trace, error) {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		return parseJSON(b)
	}
	return parseText(b)
}
func stats(h *trace.Hop, m map[string]*float64) error {
	h.Loss = m["Loss%"]
	h.Best = m["Best"]
	h.Avg = m["Avg"]
	h.Worst = m["Wrst"]
	h.Stdev = m["StDev"]
	if n := m["Snt"]; n != nil {
		if *n != math.Trunc(*n) || *n < 0 || *n > 1e9 {
			return fmt.Errorf("invalid sent count")
		}
		h.Sent = trace.Ptr(int(*n))
	}
	// A rounded percentage does not establish an exact lost count.
	if h.Sent != nil && h.Loss != nil && (*h.Loss == 0 || *h.Loss == 100) {
		h.Lost = trace.Ptr(int(*h.Loss / 100 * float64(*h.Sent)))
	}
	if h.Sent != nil && *h.Sent == 0 {
		h.Loss = nil
		h.Lost = nil
		h.Best = nil
		h.Avg = nil
		h.Worst = nil
		h.Stdev = nil
	}
	if h.Loss != nil && *h.Loss == 100 {
		h.Best = nil
		h.Avg = nil
		h.Worst = nil
		h.Stdev = nil
	}
	return nil
}
func host(h *trace.Hop, s string) {
	a := parseutil.Addresses(s)
	for _, ip := range a {
		h.Addresses = trace.AddUnique(h.Addresses, ip)
	}
	if before, _, ok := strings.Cut(s, " ("); ok {
		h.Names = trace.AddUnique(h.Names, before)
	}
	if len(a) == 0 && s != "???" && s != "" {
		h.Names = trace.AddUnique(h.Names, s)
	}
}

// parseJSON validates each row next to its source keys to preserve missing-field semantics.
func parseJSON(b []byte) (trace.Trace, error) {
	var doc struct {
		Report *struct {
			MTR struct {
				Dst   string `json:"dst"`
				Src   string `json:"src"`
				Tests *int   `json:"tests"`
			} `json:"mtr"`
			Hubs []map[string]json.RawMessage `json:"hubs"`
		} `json:"report"`
	}
	t := trace.Trace{Complete: true}
	if e := json.Unmarshal(b, &doc); e != nil {
		return t, e
	}
	if doc.Report == nil {
		return t, fmt.Errorf("missing report")
	}
	t.Dest = doc.Report.MTR.Dst
	if a, e := netip.ParseAddr(t.Dest); e == nil {
		t.DestAddr = a.Unmap().String()
	}
	if a, e := netip.ParseAddr(doc.Report.MTR.Src); e == nil {
		t.SourceAddr = a.String()
	}
	t.Probe.Cycles = doc.Report.MTR.Tests
	for i, row := range doc.Report.Hubs {
		var ttl int
		if e := json.Unmarshal(row["count"], &ttl); e != nil {
			return t, fmt.Errorf("hub %d missing count", i)
		}
		h := trace.NewHop(ttl, fmt.Sprintf("json:/report/hubs/%d", i))
		for _, key := range []string{"host", "addr", "hostname"} {
			if v, ok := row[key]; ok {
				var s string
				if e := json.Unmarshal(v, &s); e != nil {
					return t, e
				}
				host(&h, s)
			}
		}
		vals := map[string]*float64{}
		for _, k := range []string{"Snt", "Loss%", "Best", "Avg", "Wrst", "StDev"} {
			if v, ok := row[k]; ok {
				var f *float64
				if e := json.Unmarshal(v, &f); e != nil {
					return t, e
				}
				vals[k] = f
			}
		}
		if e := stats(&h, vals); e != nil {
			return t, e
		}
		t.Hops = append(t.Hops, h)
	}
	return t, nil
}

var rowRE = regexp.MustCompile(`^\s*(\d+)\.(?:\|--\s*|\s+)(.*)$`)

// parseText scans the report once; header columns and continuation rows share state.
func parseText(b []byte) (trace.Trace, error) {
	t := trace.Trace{}
	var columns []string
	for i, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Start: ") {
			raw := strings.TrimPrefix(line, "Start: ")
			if stamp, e := time.Parse(time.RFC3339, raw); e == nil {
				t.StartedAt = trace.Ptr(stamp.UTC().Format(time.RFC3339Nano))
			} else {
				t.Notes = append(t.Notes, "Imported measurement start (timezone/format unverified): "+raw)
			}
			continue
		}
		if strings.Contains(line, "Loss%") && strings.Contains(line, "Snt") {
			f := strings.Fields(line)
			for n, v := range f {
				if v == "Loss%" {
					columns = f[n:]
					break
				}
			}
			continue
		}
		m := rowRE.FindStringSubmatch(line)
		if m == nil {
			if len(t.Hops) > 0 && strings.TrimSpace(line) != "" {
				h := &t.Hops[len(t.Hops)-1]
				a := parseutil.Addresses(line)
				for _, v := range a {
					h.Addresses = trace.AddUnique(h.Addresses, v)
				}
				h.Notes = append(h.Notes, fmt.Sprintf("Continuation retained at line:%d", i+1))
			}
			continue
		}
		f := strings.Fields(m[2])
		if len(columns) == 0 || len(f) <= len(columns) {
			return t, fmt.Errorf("malformed MTR row %d", i+1)
		}
		split := len(f) - len(columns)
		h := trace.NewHop(parseutil.Int(m[1]), fmt.Sprintf("line:%d", i+1))
		name := strings.Join(f[:split], " ")
		if strings.HasPrefix(name, "AS") {
			parts := strings.Fields(name)
			if len(parts) > 1 {
				h.Notes = append(h.Notes, "Imported ASN: "+parts[0])
				name = strings.Join(parts[1:], " ")
			}
		}
		host(&h, name)
		vals := map[string]*float64{}
		for j, k := range columns {
			s := strings.TrimSuffix(f[split+j], "%")
			if s == "???" || s == "-" {
				continue
			}
			v, e := parseutil.Float(s)
			if e != nil {
				return t, e
			}
			vals[k] = trace.Ptr(v)
		}
		if e := stats(&h, vals); e != nil {
			return t, e
		}
		t.Hops = append(t.Hops, h)
	}
	t.Notes = append(t.Notes, "MTR text has no reliable destination/completeness metadata; use --dest when known.")
	return t, nil
}
