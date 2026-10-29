package explain

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/ui"
	"github.com/amb1s1/tracefull/internal/verdict"
)

func New() *cobra.Command {
	var plain bool
	c := &cobra.Command{Use: "explain", Short: "Explain implemented verdict rules and limitations", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		var b strings.Builder
		b.WriteString("# Saved-trace interpretation\n\nLoss patterns require at least 100 sent samples at every compared hop. This is not a statistical confidence guarantee. Unknown samples, destination status, missing hops and mixed responders limit comparisons.\n\n")
		for _, r := range verdict.Rules() {
			fmt.Fprintf(&b, "## %s (%s)\n\n%s\n\nExample: %s\n\n", r.Kind, r.Severity, r.Text, r.Example)
		}
		return ui.Markdown(c.OutOrStdout(), b.String(), ui.Color(c.OutOrStdout(), plain))
	}}
	c.Flags().BoolVar(&plain, "no-color", false, "Print plain Markdown")
	return c
}
