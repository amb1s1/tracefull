package tracert

import (
	"bytes"

	"github.com/amb1s1/tracefull/internal/parse/traceroute"
	"github.com/amb1s1/tracefull/internal/trace"
)

type Parser struct{}

func (Parser) Name() string                        { return "tracert" }
func (Parser) Detect(b []byte) bool                { return bytes.Contains(b, []byte("Tracing route to ")) }
func (Parser) Parse(b []byte) (trace.Trace, error) { return traceroute.ParseText(b, true) }
