package inventory

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/app"
	"github.com/amb1s1/tracefull/internal/inventory"
	"github.com/amb1s1/tracefull/internal/report"
)

func New() *cobra.Command {
	c := &cobra.Command{Use: "inventory", Short: "Inspect local inventory files"}
	c.AddCommand(&cobra.Command{Use: "check FILE", Short: "Validate inventory and report ambiguous addresses", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		info, e := os.Stat(args[0])
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Size() > app.MaxInputBytes {
			return fmt.Errorf("inventory must be a regular file of at most 16 MiB")
		}
		b, e := os.ReadFile(args[0])
		if e != nil {
			return e
		}
		inv, w, e := inventory.Load(b)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintf(c.OutOrStdout(), "Valid inventory: %d devices\n", len(inv.Devices)); e != nil {
			return e
		}
		for _, s := range w {
			if _, e = fmt.Fprintln(c.OutOrStdout(), "Warning:", report.Safe(s)); e != nil {
				return e
			}
		}
		return nil
	}})
	return c
}
