package mtr

import (
	"reflect"
	"testing"

	"github.com/amb1s1/tracefull/internal/verdict"
)

func TestContinuationEvidence(t *testing.T) {
	// Synthetic wide report: the second responder is on a continuation row.
	input := "HOST: source Loss% Snt Last Avg Best Wrst StDev\n1.|-- 192.0.2.1 0.0% 3 1 1 1 1 0\n       192.0.2.2\n       extension unavailable\n"
	got, err := (Parser{}).Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Hops[0].Evidence, []string{"line:2", "line:3"}) {
		t.Fatalf("missing continuation attribution: %v", got.Hops[0].Evidence)
	}
	fs := verdict.Evaluate(got, 0)
	if len(fs) != 1 || fs[0].Kind != "ecmp" || !reflect.DeepEqual(fs[0].Evidence, got.Hops[0].Evidence) {
		t.Fatalf("finding lost responder evidence: %+v", fs)
	}
}
