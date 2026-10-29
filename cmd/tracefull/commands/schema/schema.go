package schema

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/schema"
)

func New() *cobra.Command {
	kind := "report"
	c := &cobra.Command{Use: "schema", Short: "Print a version 1 JSON schema", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		data := schema.Report
		if kind == "investigation" {
			data = schema.Investigation
		} else if kind != "report" {
			return fmt.Errorf("unknown schema kind %q", kind)
		}
		_, err := c.OutOrStdout().Write(data)
		return err
	}}
	c.Flags().StringVar(&kind, "kind", kind, "report or investigation")
	return c
}
