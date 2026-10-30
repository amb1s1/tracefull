package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/amb1s1/tracefull/internal/collect"
	"github.com/amb1s1/tracefull/internal/report"
)

func LiveCollection(w io.Writer, format string, disabled bool) bool {
	f, ok := w.(*os.File)
	return ok && !disabled && format == "text" && os.Getenv("CI") == "" && os.Getenv("TERM") != "dumb" && term.IsTerminal(f.Fd()) && term.IsTerminal(os.Stdin.Fd())
}

type collectionOutcome struct {
	report collect.Report
	err    error
}
type collectionModel struct {
	ctx                                   context.Context
	cancel                                context.CancelFunc
	events                                <-chan collect.Event
	results                               <-chan collectionOutcome
	spinner                               spinner.Model
	viewport                              viewport.Model
	statuses                              map[string]collect.Event
	started                               time.Time
	done, aborted, filtered, showEvidence bool
	result                                collectionOutcome
}

func (m collectionModel) next() tea.Msg {
	select {
	case e := <-m.events:
		return e
	case r := <-m.results:
		return r
	case <-m.ctx.Done():
		return tea.Quit()
	}
}
func (m collectionModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.next) }
func (m collectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(max(20, msg.Width))
		m.viewport.SetHeight(max(3, msg.Height-4))
	case collect.Event:
		m.statuses[msg.Device] = msg
		return m, m.next
	case collectionOutcome:
		m.result = msg
		m.done = true
		m.setReport()
		if m.aborted {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.done {
				return m, tea.Quit
			}
			m.aborted = true
			m.cancel()
			return m, nil
		case "/":
			if m.done {
				m.filtered = !m.filtered
				m.setReport()
				return m, nil
			}
		case "e":
			if m.done {
				m.showEvidence = !m.showEvidence
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	if m.done {
		m.viewport, cmd = m.viewport.Update(msg)
	} else {
		m.spinner, cmd = m.spinner.Update(msg)
	}
	return m, cmd
}
func (m *collectionModel) setReport() {
	var b strings.Builder
	if m.filtered {
		for _, f := range m.result.report.Findings {
			fmt.Fprintf(&b, "%s: %s\n", report.Safe(f.Device), report.Safe(f.Text))
		}
		if b.Len() == 0 {
			b.WriteString("No findings.\n")
		}
	} else {
		_ = collect.Render(&b, m.result.report, "text")
	}
	m.viewport.SetContent(b.String())
}
func (m collectionModel) View() tea.View {
	var b strings.Builder
	if m.done {
		b.WriteString(m.viewport.View())
		b.WriteString("\n↑/↓ scroll · / findings · e evidence path · q return to shell\n")
		if m.showEvidence {
			b.WriteString(report.Safe(m.result.report.EvidenceDir))
		}
	} else {
		fmt.Fprintf(&b, "%s Collecting — %.0fs (q/ctrl+c cancels and preserves evidence)\n", m.spinner.View(), time.Since(m.started).Seconds())
		names := make([]string, 0, len(m.statuses))
		for name := range m.statuses {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			e := m.statuses[name]
			fmt.Fprintf(&b, "%s: %s %s\n", report.Safe(name), report.Safe(e.Command), report.Safe(e.Status))
		}
		if m.aborted {
			b.WriteString("Stopping sessions and finalizing evidence…\n")
		}
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// Collect owns the producer, cancellation and join. Bubble Tea never owns device
// resources. Even a failed UI is followed by bounded collection cleanup.
func Collect(ctx context.Context, w io.Writer, p collect.Prepared, factory collect.Factory) (collect.Report, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan collect.Event, 64)
	results := make(chan collectionOutcome, 1)
	joined := make(chan collectionOutcome, 1)
	p.Options.Events = events
	go func() {
		r, err := collect.Run(runCtx, p, factory)
		out := collectionOutcome{r, err}
		results <- out
		joined <- out
	}()
	model := collectionModel{ctx: ctx, cancel: cancel, events: events, results: results, spinner: spinner.New(spinner.WithSpinner(spinner.Line)), viewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(20)), statuses: map[string]collect.Event{}, started: time.Now()}
	for _, dp := range p.Plan.Devices {
		model.statuses[dp.Device.Name] = collect.Event{Device: dp.Device.Name, Status: "pending"}
	}
	_, uiErr := tea.NewProgram(model, tea.WithContext(ctx), tea.WithOutput(w), tea.WithInput(os.Stdin), tea.WithoutSignalHandler()).Run()
	cancel()
	out := <-joined
	if out.err != nil {
		return out.report, out.err
	}
	return out.report, uiErr
}
