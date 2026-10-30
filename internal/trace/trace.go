// Package trace contains the saved measurement model. Parsers return owned data.
package trace

import (
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strings"
	"unicode"
)

type Probe struct {
	Tool   string `json:"tool"`
	Type   string `json:"type"`
	Flows  *int   `json:"flows"`
	Cycles *int   `json:"cycles"`
}
type Match struct {
	Device     string   `json:"device"`
	Method     string   `json:"method"`
	Candidates []string `json:"candidates"`
}
type Hop struct {
	TTL       int                 `json:"ttl"`
	Addresses []string            `json:"addresses"`
	Names     []string            `json:"names"`
	RDNS      map[string][]string `json:"rdns"`
	ASN       map[string][]string `json:"asn"`
	Matches   map[string]Match    `json:"matches"`
	Sent      *int                `json:"sent"`
	Lost      *int                `json:"lost"`
	Loss      *float64            `json:"loss_pct"`
	Best      *float64            `json:"best"`
	Avg       *float64            `json:"avg"`
	Worst     *float64            `json:"worst"`
	Stdev     *float64            `json:"stdev"`
	PMTU      *int                `json:"pmtu"`
	Tags      []string            `json:"tags"`
	Notes     []string            `json:"notes"`
	Evidence  []string            `json:"evidence"`
}
type Path struct {
	SourcePort *int    `json:"source_port"`
	DestPort   *int    `json:"dest_port"`
	StartedAt  *string `json:"started_at"`
	EndedAt    *string `json:"ended_at"`
	ID         string  `json:"id"`
	Hops       []Hop   `json:"hops"`
	Reached    *bool   `json:"dest_reached"`
}
type Trace struct {
	EndedAt    *string  `json:"ended_at"`
	Dest       string   `json:"dest"`
	DestAddr   string   `json:"dest_addr"`
	SourceAddr string   `json:"source_addr"`
	StartedAt  *string  `json:"started_at"`
	Probe      Probe    `json:"probe"`
	Reached    *bool    `json:"dest_reached"`
	MaxHops    *int     `json:"max_hops"`
	Complete   bool     `json:"complete"`
	Notes      []string `json:"notes"`
	Hops       []Hop    `json:"hops"`
	Paths      []Path   `json:"paths"`
}

func Ptr[T any](v T) *T { return &v }
func NewHop(ttl int, ref string) Hop {
	return Hop{TTL: ttl, Addresses: []string{}, Names: []string{}, RDNS: map[string][]string{}, ASN: map[string][]string{}, Matches: map[string]Match{}, Tags: []string{}, Notes: []string{}, Evidence: []string{ref}}
}
func AddUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}
func (h *Hop) AddAddress(s string) bool {
	a, e := netip.ParseAddr(s)
	if e != nil {
		return false
	}
	h.Addresses = AddUnique(h.Addresses, a.Unmap().String())
	return true
}
func (h *Hop) Samples(rtts []float64, lost int) {
	sent := len(rtts) + lost
	h.Sent = Ptr(sent)
	h.Lost = Ptr(lost)
	if sent > 0 {
		h.Loss = Ptr(100 * float64(lost) / float64(sent))
	}
	if len(rtts) == 0 {
		return
	}
	best, worst, sum := rtts[0], rtts[0], 0.0
	for _, r := range rtts {
		best = math.Min(best, r)
		worst = math.Max(worst, r)
		sum += r
	}
	avg := sum / float64(len(rtts))
	variance := 0.0
	for _, r := range rtts {
		variance += (r - avg) * (r - avg)
	}
	h.Best = Ptr(best)
	h.Avg = Ptr(avg)
	h.Worst = Ptr(worst)
	h.Stdev = Ptr(math.Sqrt(variance / float64(len(rtts))))
}
func Finish(t Trace) (Trace, error) {
	if err := addressControls(t); err != nil {
		return t, err
	}
	if len(t.Hops) == 0 {
		return t, fmt.Errorf("no usable hops")
	}
	if t.Notes == nil {
		t.Notes = []string{}
	}
	if t.Paths == nil {
		t.Paths = []Path{}
	}
	sort.SliceStable(t.Hops, func(i, j int) bool { return t.Hops[i].TTL < t.Hops[j].TTL })
	prev := 0
	unresolvedIdentity := false
	for i := range t.Hops {
		h := &t.Hops[i]
		if h.TTL < 1 || h.TTL > 255 || h.TTL == prev {
			return t, fmt.Errorf("invalid or duplicate TTL %d", h.TTL)
		}
		if h.TTL > prev+1 {
			t.Notes = AddUnique(t.Notes, "Missing TTLs: comparisons across gaps are unavailable.")
		}
		prev = h.TTL
		sort.Strings(h.Addresses)
		unresolvedIdentity = unresolvedIdentity || (len(h.Addresses) == 0 && len(h.Names) > 0) || (h.Sent != nil && *h.Sent == 0)
		if h.Sent != nil && *h.Sent < 0 {
			return t, fmt.Errorf("negative sample count")
		}
		if h.Lost != nil && (*h.Lost < 0 || h.Sent == nil || *h.Lost > *h.Sent) {
			return t, fmt.Errorf("invalid lost count")
		}
		for _, v := range []*float64{h.Loss, h.Best, h.Avg, h.Worst, h.Stdev} {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
				return t, fmt.Errorf("invalid measurement")
			}
		}
		if h.Loss != nil && *h.Loss > 100 {
			return t, fmt.Errorf("loss exceeds 100 percent")
		}
		if h.Best != nil && h.Avg != nil && h.Worst != nil && (*h.Best > *h.Avg || *h.Avg > *h.Worst) {
			return t, fmt.Errorf("inconsistent RTT statistics")
		}
		for _, a := range h.Addresses {
			if a == t.DestAddr && t.DestAddr != "" && (h.Sent == nil || *h.Sent > 0) && (h.Loss == nil || *h.Loss < 100) {
				t.Reached = Ptr(true)
			}
		}
	}
	if t.Dest == "" {
		t.Dest = t.DestAddr
	}
	if t.Reached == nil && t.Complete && t.DestAddr != "" && !unresolvedIdentity {
		t.Reached = Ptr(false)
	}
	return t, nil
}

// netip permits arbitrary zone strings, including control characters. Keep those
// bytes in raw evidence rather than accepting them as normalized IP identity.
func addressControls(t Trace) error {
	values := []string{t.DestAddr, t.SourceAddr}
	add := func(hops []Hop) {
		for _, h := range hops {
			values = append(values, h.Addresses...)
		}
	}
	add(t.Hops)
	for _, p := range t.Paths {
		add(p.Hops)
	}
	for _, s := range values {
		if strings.ContainsFunc(s, unicode.IsControl) {
			return fmt.Errorf("control character in IP address")
		}
	}
	return nil
}
