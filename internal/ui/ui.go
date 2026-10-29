// Package ui contains terminal-only presentation. Domain packages do not import it.
package ui

import (
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/term"

	"github.com/amb1s1/tracefull/internal/report"
)

func Color(w io.Writer, disabled bool) bool {
	f, ok := w.(*os.File)
	_, noColor := os.LookupEnv("NO_COLOR")
	return ok && !disabled && !noColor && os.Getenv("TERM") != "dumb" && term.IsTerminal(f.Fd())
}

type theme struct{ heading, problem, warn, info, muted lipgloss.Style }

func newTheme(dark bool) theme {
	color := lipgloss.LightDark(dark)
	style := func(light, dark string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(color(lipgloss.Color(light), lipgloss.Color(dark)))
	}
	return theme{style("#005f87", "#87d7ff").Bold(true), style("#af0000", "#ff8787"), style("#875f00", "#ffd75f"), style("#444444", "#bcbcbc"), style("#666666", "#808080")}
}
func darkBackground(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(os.Stdin.Fd()) {
		return true
	}
	return lipgloss.HasDarkBackground(os.Stdin, f)
}
func Report(w io.Writer, r report.Report, format string, color bool) error {
	if !color || format != "text" {
		return report.Render(w, r, format)
	}
	theme := newTheme(darkBackground(w))
	var plain strings.Builder
	renderTable := func(headers []string, rows [][]string) string {
		return table.New().Headers(headers...).Rows(rows...).Border(lipgloss.RoundedBorder()).StyleFunc(func(row, col int) lipgloss.Style {
			if row < 0 {
				return theme.heading
			}
			return lipgloss.NewStyle()
		}).String()
	}
	if e := report.RenderText(&plain, r, renderTable); e != nil {
		return e
	}
	lines := strings.Split(plain.String(), "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "Destination:"), strings.HasPrefix(line, "Device "), line == "Findings":
			lines[i] = theme.heading.Render(line)
		case strings.HasPrefix(line, "- problem"):
			lines[i] = theme.problem.Render(line)
		case strings.HasPrefix(line, "- warn"):
			lines[i] = theme.warn.Render(line)
		case strings.HasPrefix(line, "- info"):
			lines[i] = theme.info.Render(line)
		case strings.Contains(line, "private"), strings.HasPrefix(line, "Note:"):
			lines[i] = theme.muted.Render(line)
		}
	}
	_, e := io.WriteString(w, strings.Join(lines, "\n"))
	return e
}
func Markdown(w io.Writer, s string, color bool) error {
	if color {
		r, e := glamour.NewTermRenderer(glamour.WithStandardStyle(backgroundStyle(w)), glamour.WithWordWrap(90))
		if e != nil {
			return e
		}
		s, e = r.Render(s)
		if e != nil {
			return e
		}
	}
	_, e := io.WriteString(w, s)
	return e
}

func backgroundStyle(w io.Writer) string {
	if darkBackground(w) {
		return "dark"
	}
	return "light"
}
