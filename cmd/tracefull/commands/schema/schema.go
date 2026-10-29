package schema

import (
	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/schema"
)

func New() *cobra.Command {
	return &cobra.Command{Use: "schema", Short: "Print the version 1 report JSON schema", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error { _, e := c.OutOrStdout().Write(schema.Report); return e }}
}
