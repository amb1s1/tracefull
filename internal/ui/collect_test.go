package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/amb1s1/tracefull/internal/collect"
)

func TestCollectionModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := collectionModel{ctx: ctx, cancel: cancel, statuses: map[string]collect.Event{}, spinner: spinner.New(), viewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)), started: time.Now()}
	next, _ := m.Update(collect.Event{Device: "switch\x1b[31m", Command: "route", Status: "running"})
	m = next.(collectionModel)
	if strings.Contains(m.View().Content, "\x1b[31m") || !strings.Contains(m.View().Content, "route running") {
		t.Fatal(m.View().Content)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'q'})
	m = next.(collectionModel)
	if ctx.Err() == nil || !m.aborted {
		t.Fatal("q did not cancel")
	}
	next, cmd := m.Update(collectionOutcome{report: collect.Report{Status: "interrupted"}})
	m = next.(collectionModel)
	if !m.done || cmd == nil {
		t.Fatal("aborted run did not quit after result")
	}
}
func TestLiveDisabledForPipes(t *testing.T) {
	var out strings.Builder
	if LiveCollection(&out, "text", false) {
		t.Fatal("live output on pipe")
	}
}
