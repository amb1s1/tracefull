package collect

import (
	"fmt"
	"io"

	"github.com/amb1s1/tracefull/internal/collect"
	"github.com/amb1s1/tracefull/internal/config"
	"github.com/amb1s1/tracefull/internal/ui"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	o := collect.Defaults()
	var dryRun, jsonOut, markdown, live, noLive bool
	format := "text"
	c := &cobra.Command{Use: "collect --from FILE|-", Short: "Collect attributed read-only device evidence from a saved trace", Args: cobra.NoArgs, Example: "  tracefull collect --from trace.txt --inventory inventory.yaml --dry-run\n  tracefull collect --from trace.txt --inventory inventory.yaml --sudo --json"}
	f := c.Flags()
	f.StringVar(&o.From, "from", "", "Saved trace path, or - for stdin (required)")
	f.StringVar(&o.Inventory, "inventory", "", "Inventory YAML/JSON file")
	f.StringVar(&o.Destination, "dest", "", "Numeric destination override")
	f.StringArrayVar(&o.Maps, "map", nil, "Explicit hop IP=device mapping (repeatable)")
	f.StringSliceVar(&o.Groups, "collect", o.Groups, "ingress,route,counters,optics,lldp,version,cpu,reverse, or all")
	f.StringArrayVar(&o.Extras, "extra-cmd", nil, "Extra command within the platform read-only policy (repeatable)")
	f.BoolVar(&o.OwnerSweep, "owner-sweep", false, "Discover unmatched hop owners with bounded inventory address reads")
	f.StringVar(&o.Site, "site", "", "Limit owner sweep to this inventory site")
	f.StringVar(&o.Role, "role", "", "Limit owner sweep to this inventory role")
	f.StringVar(&o.VRF, "vrf", "default", "Routing context; unvalidated contexts fail without fallback")
	f.StringVar(&o.User, "user", "", "SSH user (inventory default, then local user)")
	f.StringVar(&o.Jump, "jump", "", "One SSH jump host; same user and authentication")
	f.StringArrayVar(&o.Identities, "identity", nil, "Private key file (repeatable; no password prompts)")
	f.StringVar(&o.KnownHosts, "known-hosts", "", "SSH known_hosts path (default ~/.ssh/known_hosts)")
	f.BoolVar(&o.InsecureHostKey, "insecure-host-key", false, "Explicitly disable SSH host-key verification")
	f.StringVar(&o.Wrapper, "ssh-cmd", "", "Wrapper argv template; {cmd} must be a single argument")
	f.BoolVar(&o.Sudo, "sudo", false, "Use sudo -n for the Cumulus FRR and LLDP read commands")
	f.DurationVar(&o.ConnectTimeout, "connect-timeout", o.ConnectTimeout, "SSH connection deadline")
	f.DurationVar(&o.CommandTimeout, "cmd-timeout", o.CommandTimeout, "Per-command deadline")
	f.DurationVar(&o.Timeout, "timeout", o.Timeout, "Overall collection deadline (maximum 30m)")
	f.IntVar(&o.Workers, "workers", o.Workers, "Parallel devices (1–32); commands per device remain serial")
	f.StringVar(&o.EvidenceDir, "evidence-dir", "", "New private evidence directory; default creates a hidden unique directory here")
	f.BoolVar(&live, "live", true, "Interactive collection view when text output and terminal input/output allow it")
	f.BoolVar(&noLive, "no-live", false, "Disable the interactive collection view")
	f.BoolVar(&dryRun, "dry-run", false, "Offline command plan; no DNS, SSH, wrappers or evidence writes")
	f.StringVarP(&format, "format", "o", format, "text, json or markdown")
	f.BoolVar(&jsonOut, "json", false, "JSON output")
	f.BoolVar(&markdown, "markdown", false, "Markdown output")
	c.RunE = func(c *cobra.Command, _ []string) error {
		if err := ui.Overlay(c, config.UserDir(), "."); err != nil {
			return err
		}
		if f.Changed("live") && live && noLive {
			return fmt.Errorf("--live and --no-live conflict")
		}
		if jsonOut && markdown || f.Changed("format") && ((jsonOut && format != "json") || (markdown && format != "markdown")) {
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
		input := c.InOrStdin()
		stdin, ok := input.(io.ReadCloser)
		if !ok {
			stdin = io.NopCloser(input)
		}
		p, err := collect.Prepare(c.Context(), o, stdin)
		if err != nil {
			return err
		}
		if dryRun {
			return collect.RenderPlan(c.OutOrStdout(), p.Plan, format)
		}
		var r collect.Report
		if ui.LiveCollection(c.OutOrStdout(), format, noLive || !live || o.From == "-") {
			r, err = ui.Collect(c.Context(), c.OutOrStdout(), p, collect.SystemFactory(p.Options))
		} else {
			r, err = collect.Run(c.Context(), p, collect.SystemFactory(p.Options))
		}
		if r.EvidenceDir != "" && c.Context().Err() == nil {
			if e := collect.Render(c.OutOrStdout(), r, format); e != nil {
				return e
			}
		}
		return err
	}
	return c
}
