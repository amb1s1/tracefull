package tracepath

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string { return "tracepath" }
func (Parser) Detect(b []byte) bool {
	return bytes.Contains(b, []byte("pmtu ")) || bytes.Contains(b, []byte("Resume:")) || bytes.Contains(b, []byte("no reply"))
}

var mtuRE = regexp.MustCompile(`pmtu (\d+)`)

// Parse keeps PMTU retries in one scan because repeated TTL rows update prior evidence.
func (Parser) Parse(b []byte) (trace.Trace, error) {
	t := trace.Trace{}
	var mtu *int
	byTTL := map[int]int{}
	for i, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "Resume:") {
			t.Complete = true
			continue
		}
		if n := mtuRE.FindStringSubmatch(line); n != nil {
			mtu = trace.Ptr(parseutil.Int(n[1]))
		}
		m := parseutil.HopLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ttl := parseutil.Int(m[1])
		body := m[2]
		if n := mtuRE.FindStringSubmatch(body); n != nil {
			mtu = trace.Ptr(parseutil.Int(n[1]))
		}
		if strings.Contains(body, "[LOCALHOST]") {
			continue
		}
		var h trace.Hop
		var e error
		if strings.Contains(body, "no reply") {
			h = trace.NewHop(ttl, fmt.Sprintf("line:%d", i+1))
			h.Notes = append(h.Notes, "no reply; sent count unknown")
		} else {
			h, e = parseutil.Hop(ttl, body, fmt.Sprintf("line:%d", i+1))
			if e != nil {
				return t, e
			}
			h.Sent = nil
			h.Lost = nil
			h.Loss = nil
		}
		h.PMTU = mtu
		if strings.Contains(body, "asymm") {
			h.Tags = append(h.Tags, "asymmetry-hint")
		}
		if strings.Contains(body, "reached") {
			t.Reached = trace.Ptr(true)
			t.Complete = true
			if len(h.Addresses) == 1 {
				t.DestAddr = h.Addresses[0]
			}
		}
		if idx, ok := byTTL[ttl]; ok {
			old := &t.Hops[idx]
			old.PMTU = mtu
			old.Evidence = append(old.Evidence, h.Evidence...)
			for _, a := range h.Addresses {
				old.Addresses = trace.AddUnique(old.Addresses, a)
			}
			old.Notes = append(old.Notes, "Repeated TTL includes PMTU retry; statistics not merged.")
		} else {
			byTTL[ttl] = len(t.Hops)
			t.Hops = append(t.Hops, h)
		}
	}
	return t, nil
}
