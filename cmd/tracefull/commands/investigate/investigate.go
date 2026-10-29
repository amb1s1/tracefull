package investigate

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amb1s1/tracefull/internal/investigate"
)

func New() *cobra.Command {
	o := investigate.Defaults()
	var dryRun, jsonOut, markdown, noRDNS bool
	format := "text"
	c := &cobra.Command{Use: "investigate TARGET", Short: "Run local diagnostic tools and report their attributed observations", Args: cobra.ExactArgs(1), Example: "  tracefull investigate 1.1.1.1 --dry-run\n  tracefull investigate 1.1.1.1 --tools traceroute,mtr"}
	f := c.Flags()
	f.StringSliceVar(&o.Tools, "tools", nil, "Tool selection: traceroute,mtr,tracepath (default: all available)")
	f.BoolVar(&dryRun, "dry-run", false, "Show the plan without DNS, execution or evidence writes")
	f.IntVar(&o.MaxHops, "max-hops", o.MaxHops, "Maximum TTL (1–255)")
	f.IntVar(&o.Cycles, "cycles", o.Cycles, "MTR cycles (1–1000)")
	f.DurationVar(&o.Timeout, "timeout", o.Timeout, "Overall execution deadline (maximum 30m)")
	f.DurationVar(&o.ToolTimeout, "tool-timeout", o.ToolTimeout, "Deadline for each diagnostic tool")
	f.StringVar(&o.EvidenceDir, "evidence-dir", "", "New evidence directory; default creates a unique directory here")
	f.StringVar(&o.Inventory, "inventory", "", "Optional inventory YAML/JSON file")
	f.BoolVar(&o.RDNS, "rdns", false, "Opt in to reverse DNS enrichment")
	f.BoolVar(&noRDNS, "no-rdns", false, "Explicitly disable reverse DNS enrichment")
	f.BoolVar(&o.ASN, "asn", false, "Opt in to ASN DNS enrichment")
	f.StringVarP(&format, "format", "o", format, "text, json or markdown")
	f.BoolVar(&jsonOut, "json", false, "JSON output")
	f.BoolVar(&markdown, "markdown", false, "Markdown output")
	c.RunE = func(c *cobra.Command, args []string) error {
		o.Target = args[0]
		if jsonOut && markdown || c.Flags().Changed("format") && ((jsonOut && format != "json") || (markdown && format != "markdown")) {
			return fmt.Errorf("conflicting output formats")
		}
		if jsonOut {
			format = "json"
		}
		if markdown {
			format = "markdown"
		}
		if format != "text" && format != "json" && format != "markdown" {
			return fmt.Errorf("invalid format %q", format)
		}
		if noRDNS && o.RDNS {
			return fmt.Errorf("--rdns and --no-rdns conflict")
		}
		d := investigate.System()
		if dryRun {
			p, err := investigate.BuildPlan(o, d.GOOS, d.Lookup)
			if err != nil {
				return err
			}
			return investigate.RenderPlan(c.OutOrStdout(), p, format)
		}
		r, err := investigate.Run(c.Context(), o, d)
		if r.EvidenceDir != "" && c.Context().Err() == nil {
			if outputErr := investigate.Render(c.OutOrStdout(), r, format); outputErr != nil {
				return outputErr
			}
		}
		return err
	}
	return c
}
