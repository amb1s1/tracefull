package traceroute

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string { return "traceroute" }
func (Parser) Detect(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "traceroute to ") || numberedHop.MatchString(strings.Split(s, "\n")[0])
}
func (Parser) Parse(b []byte) (trace.Trace, error) { return ParseText(b, false) }

// Unlike tracepath's colon-prefixed TTLs, traceroute TTLs require whitespace.
// Otherwise an indented IPv6 responder can be mistaken for a new hop number.
var numberedHop = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)

type hopGroup struct {
	ttl      int
	body     strings.Builder
	evidence []string
}

// ParseText groups wrapped responder rows before computing probe statistics.
// Keeping the scan together preserves extension-line warnings and source order.
func ParseText(b []byte, windows bool) (trace.Trace, error) {
	t := trace.Trace{}
	var group *hopGroup
	flush := func() error {
		if group == nil {
			return nil
		}
		h, e := parseutil.Hop(group.ttl, group.body.String(), group.evidence[0])
		if e != nil {
			return e
		}
		h.Evidence = group.evidence
		t.Hops = append(t.Hops, h)
		group = nil
		return nil
	}
	for n, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parseutil.Header(&t, line)
		if line == "Trace complete." {
			if e := flush(); e != nil {
				return t, e
			}
			t.Complete = true
			continue
		}
		ref := fmt.Sprintf("line:%d", n+1)
		if m := numberedHop.FindStringSubmatch(line); m != nil {
			if e := flush(); e != nil {
				return t, e
			}
			group = &hopGroup{ttl: parseutil.Int(m[1]), evidence: []string{ref}}
			group.body.WriteString(m[2])
			continue
		}
		if group != nil && len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t') && continuation(line) {
			group.body.WriteByte(' ')
			group.body.WriteString(line)
			group.evidence = append(group.evidence, ref)
			continue
		}
		if strings.HasPrefix(line, "traceroute ") || strings.HasPrefix(line, "Tracing route") || strings.HasPrefix(line, "over a maximum") {
			continue
		}
		t.Notes = append(t.Notes, fmt.Sprintf("Unparsed line %d retained in source.", n+1))
	}
	if e := flush(); e != nil {
		return t, e
	}
	// Text EOF cannot establish a completed measurement: captures may be truncated.
	if !windows {
		t.Notes = append(t.Notes, "Text capture completeness and measurement window are unknown.")
	}
	return t, nil
}
func continuation(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	if fields[0] == "*" {
		return true
	}
	responder := len(parseutil.Addresses(fields[0])) > 0
	if !responder && len(fields) > 1 && strings.HasPrefix(fields[1], "(") {
		responder = len(parseutil.Addresses(fields[1])) > 0
	}
	return responder && (parseutil.RTT.MatchString(line) || strings.Contains(line, "*"))
}
