package enrich

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/amb1s1/tracefull/internal/app"
	"github.com/amb1s1/tracefull/internal/config"
	"github.com/amb1s1/tracefull/internal/ui"
	"github.com/amb1s1/tracefull/internal/verdict"
)

var ErrFinding = errors.New("configured finding present")

func New() *cobra.Command {
	var o app.Options
	var format string
	var jsonOut, markdown, noRDNS, noColor bool
	var failOn []string
	var quiet, verbose bool
	var logFormat string
	c := &cobra.Command{Use: "enrich --from FILE|-", Short: "Interpret and enrich a saved trace", Args: cobra.NoArgs, Example: "  tracefull enrich --from trace.txt --no-rdns\n  tracefull enrich --from trace.json --json --fail-on real-loss"}
	f := c.Flags()
	f.BoolVar(&quiet, "quiet", false, "Suppress diagnostic logs (errors remain visible)")
	f.BoolVarP(&verbose, "verbose", "v", false, "Write processing details to stderr")
	f.StringVar(&logFormat, "log-format", "text", "Diagnostic log format: text or json")
	f.StringVar(&o.From, "from", "", "Saved trace file, or - for stdin")
	f.StringVar(&o.Inventory, "inventory", "", "Inventory YAML/JSON (optional default discovery)")
	f.StringVar(&o.Dest, "dest", "", "Destination IP if missing from input")
	f.StringSliceVar(&o.Maps, "map", nil, "IP=DEVICE override (repeatable)")
	f.BoolVar(&o.RDNS, "rdns", false, "Opt in to reverse DNS lookups")
	f.BoolVar(&noRDNS, "no-rdns", false, "Disable inherited reverse DNS setting")
	f.BoolVar(&o.ASN, "asn", false, "Opt in to public-hop ASN DNS lookups")
	f.DurationVar(&o.Timeout, "rdns-timeout", time.Second, "Timeout for each DNS lookup")
	f.DurationVar(&o.LatencyJump, "latency-jump", verdict.DefaultLatencyJump, "RTT jump threshold")
	f.StringVarP(&format, "format", "o", "text", "text, json or markdown")
	f.BoolVar(&jsonOut, "json", false, "JSON report")
	f.BoolVar(&markdown, "markdown", false, "Markdown report")
	f.BoolVar(&noColor, "no-color", false, "Disable terminal colors")
	f.StringSliceVar(&failOn, "fail-on", nil, "Finding kinds that return exit 1")
	c.RunE = func(c *cobra.Command, _ []string) error {
		o.UserConfigDir = config.UserDir()
		var e error
		o.ProjectDir, e = os.Getwd()
		if e != nil {
			return e
		}
		if e = overlay(c, o.UserConfigDir, o.ProjectDir); e != nil {
			return e
		}
		if logFormat != "text" && logFormat != "json" {
			return fmt.Errorf("invalid log format %q", logFormat)
		}
		if c.Flags().Changed("rdns") && o.RDNS && noRDNS {
			return fmt.Errorf("--rdns and --no-rdns conflict")
		}
		if c.Flags().Changed("format") && ((jsonOut && format != "json") || (markdown && format != "markdown")) {
			return fmt.Errorf("--format and shorthand conflict")
		}
		if noRDNS {
			o.RDNS = false
		}
		if jsonOut && markdown {
			return fmt.Errorf("--json and --markdown conflict")
		}
		if jsonOut {
			format = "json"
		}
		if markdown {
			format = "markdown"
		}
		if format != "json" && format != "markdown" && format != "text" {
			return fmt.Errorf("invalid format %q", format)
		}
		for _, k := range failOn {
			found := false
			for _, r := range verdict.Rules() {
				found = found || r.Kind == k
			}
			if !found {
				return fmt.Errorf("unknown --fail-on kind %q", k)
			}
		}
		in, ok := c.InOrStdin().(io.ReadCloser)
		if !ok {
			in = io.NopCloser(c.InOrStdin())
		}
		r, e := app.Build(c.Context(), o, in, net.DefaultResolver)
		if e != nil {
			return e
		}
		if verbose && !quiet {
			var handler slog.Handler = slog.NewTextHandler(c.ErrOrStderr(), nil)
			if logFormat == "json" {
				handler = slog.NewJSONHandler(c.ErrOrStderr(), nil)
			}
			slog.New(handler).InfoContext(c.Context(), "Saved trace processed", "tool", r.Probe.Tool, "hops", len(r.Hops), "findings", len(r.Findings))
		}
		if e = ui.Report(c.OutOrStdout(), r, format, ui.Color(c.OutOrStdout(), noColor)); e != nil {
			return e
		}
		if app.CheckFindings(r, failOn) {
			return ErrFinding
		}
		return nil
	}
	return c
}
func overlay(c *cobra.Command, user, project string) error {
	values, e := config.Values(user, project)
	if e != nil {
		return e
	}
	flags := c.Flags()
	env := map[string]string{}
	flags.VisitAll(func(f *pflag.Flag) {
		key := "TRACEFULL_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v, ok := os.LookupEnv(key); ok {
			env[f.Name] = v
		}
	})
	normalized, e := config.Normalize(env)
	if e != nil {
		return e
	}
	for k, v := range normalized {
		values[k] = v
	}
	keys := []string{}
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := flags.Lookup(k)
		if f == nil || k == "help" {
			return fmt.Errorf("unknown configuration key %q", k)
		}
		if !f.Changed {
			if e := f.Value.Set(values[k]); e != nil {
				return fmt.Errorf("config %s: %w", k, e)
			}
		}
	}
	return nil
}
