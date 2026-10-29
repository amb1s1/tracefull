package root

import (
	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/cmd/tracefull/commands/enrich"
	"github.com/amb1s1/tracefull/cmd/tracefull/commands/explain"
	"github.com/amb1s1/tracefull/cmd/tracefull/commands/inventory"
	"github.com/amb1s1/tracefull/cmd/tracefull/commands/schema"
	"github.com/amb1s1/tracefull/cmd/tracefull/commands/version"
)

func New() *cobra.Command {
	c := &cobra.Command{Use: "tracefull", Short: "Interpret saved traces with attributed, qualified findings", SilenceErrors: true, SilenceUsage: true}
	c.AddCommand(enrich.New(), explain.New(), schema.New(), inventory.New(), version.New())
	return c
}
