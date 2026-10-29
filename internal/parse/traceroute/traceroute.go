package traceroute

import (
	"fmt"
	"strings"

	"github.com/amb1s1/tracefull/internal/parse/parseutil"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string { return "traceroute" }
func (Parser) Detect(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "traceroute to ") || parseutil.HopLine.MatchString(strings.Split(s, "\n")[0])
}
func (Parser) Parse(b []byte) (trace.Trace, error) { return ParseText(b, false) }
func ParseText(b []byte, windows bool) (trace.Trace, error) {
	t := trace.Trace{}
	for n, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parseutil.Header(&t, line)
		if line == "Trace complete." {
			t.Complete = true
			continue
		}
		m := parseutil.HopLine.FindStringSubmatch(line)
		if m == nil {
			if strings.HasPrefix(line, "traceroute ") || strings.HasPrefix(line, "Tracing route") || strings.HasPrefix(line, "over a maximum") {
				continue
			}
			t.Notes = append(t.Notes, fmt.Sprintf("Unparsed line %d retained in source.", n+1))
			continue
		}
		h, e := parseutil.Hop(parseutil.Int(m[1]), m[2], fmt.Sprintf("line:%d", n+1))
		if e != nil {
			return t, e
		}
		t.Hops = append(t.Hops, h)
	}
	// Text EOF cannot establish a completed measurement: captures may be truncated.
	if !windows {
		t.Notes = append(t.Notes, "Text capture completeness and measurement window are unknown.")
	}
	return t, nil
}
