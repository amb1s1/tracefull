package collect

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelRunningCollectionClosesSession(t *testing.T) {
	p := prepare(t, nil)
	s := &fakeSession{t: t, block: true, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		report Report
		err    error
	}
	done := make(chan result, 1)
	go func() { r, err := Run(ctx, p, factory(s)); done <- result{r, err} }()
	select {
	case <-s.started:
	case <-time.After(5 * time.Second):
		t.Fatal("command did not start")
	}
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.Canceled) || result.report.Status != "interrupted" || !s.closed || len(s.commands) != 1 {
			t.Fatal(result.report.Status, result.err, s.closed, s.commands)
		}
		validateReport(t, result.report)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled command did not close and finish")
	}
}

func TestExpiredCollectionDoesNotAcquireSession(t *testing.T) {
	p := prepare(t, nil)
	s := &fakeSession{t: t}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	r, err := Run(ctx, p, factory(s))
	if !errors.Is(err, context.DeadlineExceeded) || r.Status != "interrupted" || s.policy != nil || s.closed || len(s.commands) != 0 {
		t.Fatal(r.Status, err, s.closed, s.commands)
	}
	validateReport(t, r)
}
