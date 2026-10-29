package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/app"
)

func New() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the build version", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { _, e := fmt.Fprintln(c.OutOrStdout(), app.Version); return e }}
}
