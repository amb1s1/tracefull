package dublin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string         { return "dublin" }
func (Parser) Detect(b []byte) bool { return bytes.Contains(b, []byte(`"flows"`)) }

type packet struct {
	Timestamp json.RawMessage `json:"timestamp"`
	UDP       *struct {
		Source int `json:"sport"`
		Dest   int `json:"dport"`
	} `json:"udp"`
	IP struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
		TTL int    `json:"ttl"`
	} `json:"ip"`
	ICMP *struct {
		Type int `json:"type"`
		Code int `json:"code"`
	} `json:"icmp"`
}
type probe struct {
	Sent     packet   `json:"sent"`
	Received *packet  `json:"received"`
	RTT      *float64 `json:"rtt_usec"`
	Last     bool     `json:"is_last"`
	NAT      *int     `json:"nat_id"`
	Name     string   `json:"name"`
}

// Parse keeps a flow transaction together so validation cannot leave a half-merged path.
func (Parser) Parse(b []byte) (trace.Trace, error) {
	var doc struct {
		Flows map[string][]probe `json:"flows"`
	}
	t := trace.Trace{Complete: true}
	if e := json.Unmarshal(b, &doc); e != nil {
		return t, e
	}
	ids := []string{}
	for id := range doc.Flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	t.Probe.Flows = trace.Ptr(len(ids))
	t.Probe.Type = "udp"
	merged := map[int]*trace.Hop{}
	for _, id := range ids {
		p := trace.Path{ID: id, Hops: []trace.Hop{}}
		var previousNAT *int
		for j, v := range doc.Flows[id] {
			if _, e := netip.ParseAddr(v.Sent.IP.Dst); e != nil {
				return t, fmt.Errorf("invalid destination")
			}
			if t.DestAddr != "" && t.DestAddr != v.Sent.IP.Dst {
				return t, fmt.Errorf("mixed destinations")
			}
			t.DestAddr = v.Sent.IP.Dst
			t.SourceAddr = v.Sent.IP.Src
			h := trace.NewHop(v.Sent.IP.TTL, fmt.Sprintf("json:/flows/%s/%d", id, j))
			start, e := parseutil.UnixTimestamp(v.Sent.Timestamp)
			if e != nil {
				return t, e
			}
			p.StartedAt = parseutil.Earlier(p.StartedAt, start)
			if v.Sent.UDP != nil {
				if v.Sent.UDP.Source < 0 || v.Sent.UDP.Source > 65535 || v.Sent.UDP.Dest < 0 || v.Sent.UDP.Dest > 65535 {
					return t, fmt.Errorf("invalid UDP port")
				}
				if p.SourcePort != nil && (*p.SourcePort != v.Sent.UDP.Source || *p.DestPort != v.Sent.UDP.Dest) {
					return t, fmt.Errorf("changing tuple within flow %s", id)
				}
				p.SourcePort = trace.Ptr(v.Sent.UDP.Source)
				p.DestPort = trace.Ptr(v.Sent.UDP.Dest)
			}

			if v.Received == nil {
				h.Samples(nil, 1)
			} else {
				end, e := parseutil.UnixTimestamp(v.Received.Timestamp)
				if e != nil {
					return t, e
				}
				p.EndedAt = parseutil.Later(p.EndedAt, end)
				if !h.AddAddress(v.Received.IP.Src) {
					return t, fmt.Errorf("invalid response address")
				}
				if v.RTT == nil {
					return t, fmt.Errorf("missing response RTT")
				}
				h.Samples([]float64{*v.RTT / 1000}, 0)
				if v.Name != "" {
					h.Names = append(h.Names, v.Name)
				}
				if v.Last && v.Received.IP.Src == t.DestAddr {
					p.Reached = trace.Ptr(true)
				}
				if c := v.Received.ICMP; c != nil && c.Type == 3 && c.Code != 3 {
					h.Notes = append(h.Notes, fmt.Sprintf("!ICMP3/%d", c.Code))
				}
			}
			if previousNAT != nil && v.NAT != nil && *previousNAT != *v.NAT {
				h.Tags = append(h.Tags, "nat-hint")
			}
			previousNAT = v.NAT
			p.Hops = append(p.Hops, h)
			if m, ok := merged[h.TTL]; ok {
				for _, a := range h.Addresses {
					m.Addresses = trace.AddUnique(m.Addresses, a)
				}
				m.Evidence = append(m.Evidence, h.Evidence...)
			} else {
				copy := trace.NewHop(h.TTL, h.Evidence[0])
				copy.Addresses = append(copy.Addresses, h.Addresses...)
				merged[h.TTL] = &copy
			}
			if v.Last {
				break
			}
		}
		checked, e := trace.Finish(trace.Trace{Hops: p.Hops})
		if e != nil {
			return t, e
		}
		p.Hops = checked.Hops
		// The JSON container can hold a partial run; no explicit negative reachability.
		if p.Reached != nil && *p.Reached {
			t.Reached = trace.Ptr(true)
		}
		t.StartedAt = parseutil.Earlier(t.StartedAt, p.StartedAt)
		t.EndedAt = parseutil.Later(t.EndedAt, p.EndedAt)
		t.Paths = append(t.Paths, p)
	}
	for _, h := range merged {
		t.Hops = append(t.Hops, *h)
	}
	t.Complete = false
	t.Notes = append(t.Notes, "Dublin flows retained separately; aggregate hop statistics intentionally unknown.")
	return t, nil
}
