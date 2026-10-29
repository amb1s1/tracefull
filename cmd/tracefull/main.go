package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/fang"

	"github.com/amb1s1/tracefull/cmd/tracefull/commands/enrich"
	"github.com/amb1s1/tracefull/cmd/tracefull/root"
	"github.com/amb1s1/tracefull/internal/app"
	"github.com/amb1s1/tracefull/internal/investigate"
	"github.com/amb1s1/tracefull/internal/report"
	"github.com/amb1s1/tracefull/internal/ui"
)

func run() (code int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, cleanup, e := ui.Output(ctx, os.Stdout)
	if e != nil {
		fmt.Fprintln(os.Stderr, "tracefull:", report.Safe(e.Error()))
		return 2
	}
	defer func() {
		if e := cleanup(); e != nil && code == 0 {
			fmt.Fprintln(os.Stderr, "tracefull:", report.Safe(e.Error()))
			code = 2
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan struct{})
	received := make(chan os.Signal, 1)
	go func() {
		defer close(done)
		select {
		case s := <-signals:
			received <- s
			cancel()
		case <-ctx.Done():
		}
	}()

	command := root.New()
	command.SetOut(output)
	e = fang.Execute(ctx, command, fang.WithVersion(app.Version), fang.WithNotifySignal(), fang.WithErrorHandler(func(io.Writer, fang.Styles, error) {}))
	cancel()
	<-done
	select {
	case s := <-received:
		if s == syscall.SIGTERM {
			return 143
		}
		return 130
	default:
	}
	if errors.Is(e, enrich.ErrFinding) {
		return 1
	}
	if errors.Is(e, investigate.ErrNoResults) || errors.Is(e, context.DeadlineExceeded) {
		return 3
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "tracefull:", report.Safe(e.Error()))
		return 2
	}
	return 0
}
func main() { os.Exit(run()) }
