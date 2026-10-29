// Package parse defines a pure, explicitly assembled dialect registry.
package parse

import (
	"fmt"

	"github.com/amb1s1/tracefull/internal/trace"
)

type Dialect interface {
	Name() string
	Detect([]byte) bool
	Parse([]byte) (trace.Trace, error)
}
type Registry struct{ dialects []Dialect }

func New(ds ...Dialect) Registry { return Registry{append([]Dialect(nil), ds...)} }
func (r Registry) Parse(data []byte) (trace.Trace, error) {
	for _, d := range r.dialects {
		if d.Detect(data) {
			t, e := d.Parse(data)
			if e != nil {
				return t, fmt.Errorf("%s: %w", d.Name(), e)
			}
			t.Probe.Tool = d.Name()
			return trace.Finish(t)
		}
	}
	return trace.Trace{}, fmt.Errorf("unrecognized saved trace format")
}
