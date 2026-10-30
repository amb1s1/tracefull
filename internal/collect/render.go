package collect

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/amb1s1/tracefull/internal/report"
)

func encode(w io.Writer, value any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(value)
}
func escaper(format string) (func(string) string, error) {
	switch format {
	case "text":
		return report.Safe, nil
	case "markdown":
		return report.Escape, nil
	}
	return nil, fmt.Errorf("invalid format %q", format)
}
func RenderPlan(w io.Writer, p Plan, format string) error {
	if format == "json" {
		return encode(w, p)
	}
	escape, err := escaper(format)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Collection dry run: %s | context: %s\nNo DNS, SSH, wrapper execution or evidence writes.\n", escape(p.Destination), escape(p.Context))
	for _, d := range append(slices.Clone(p.Devices), p.SweepDevices...) {
		fmt.Fprintf(&b, "\nDevice %s (%s) | %s | %s\n", escape(d.Device.Name), escape(d.Transport.Address), escape(d.Transport.Kind), escape(d.Status))
		if d.Reason != "" {
			fmt.Fprintf(&b, "  %s\n", escape(d.Reason))
		}
		for _, c := range append(slices.Clone(d.Commands), d.Conditional...) {
			fmt.Fprintf(&b, "- %s: %s", escape(c.Name), escape(c.Status))
			if c.Text != "" {
				fmt.Fprintf(&b, " command=%s", escape(c.Text))
			}
			if len(c.Argv) > 0 {
				argv, _ := json.Marshal(c.Argv)
				fmt.Fprintf(&b, " argv=%s", escape(string(argv)))
			}
			if c.Reason != "" {
				fmt.Fprintf(&b, " — %s", escape(c.Reason))
			}
			b.WriteByte('\n')
		}
	}
	if len(p.Devices) == 0 && len(p.SweepDevices) == 0 {
		b.WriteString("No unambiguously matched devices.\n")
	}
	for _, n := range p.Notes {
		fmt.Fprintf(&b, "Note: %s\n", escape(n))
	}
	_, err = io.WriteString(w, b.String())
	return err
}
func Render(w io.Writer, r Report, format string) error {
	if format == "json" {
		return encode(w, r)
	}
	escape, err := escaper(format)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Collection: %s | context: %s | %s\nExecution window (UTC): %s to %s\nEvidence: %s\n", escape(r.Plan.Destination), escape(r.Plan.Context), escape(r.Status), escape(r.StartedAt), escape(r.EndedAt), escape(r.EvidenceDir))
	if r.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", escape(r.Error))
	}
	for _, d := range append(slices.Clone(r.OwnerSweep), r.Devices...) {
		fmt.Fprintf(&b, "\nDevice %s: %s | interfaces: %s\n", escape(d.Name), escape(d.Status), escape(strings.Join(d.Interfaces, ", ")))
		if d.Reason != "" {
			fmt.Fprintf(&b, "  %s\n", escape(d.Reason))
		}
		for _, c := range d.Commands {
			fmt.Fprintf(&b, "- %s: %s", escape(c.Name), escape(c.Status))
			if c.Reason != "" {
				fmt.Fprintf(&b, " — %s", escape(c.Reason))
			}
			if c.Stdout != nil {
				fmt.Fprintf(&b, " [raw: %s]", escape(c.Stdout.Path))
			}
			b.WriteByte('\n')
			if c.Facts == nil {
				continue
			}
			f := c.Facts
			for _, a := range f.Addresses {
				fmt.Fprintf(&b, "  Address %s on %s (master: %s; routing context: %s)\n", escape(a.Address), escape(a.Interface), escape(a.Master), escape(contextValue(a.RoutingContext)))
			}
			for _, route := range f.Routes {
				fmt.Fprintf(&b, "  Route %s via %s | context: %s | selected: %s | installed: %s\n", escape(route.Prefix), escape(route.Protocol), escape(route.Context), value(route.Selected), value(route.Installed))
				for _, n := range route.NextHops {
					fmt.Fprintf(&b, "    Next hop %s on %s | active: %s | installed: %s\n", escape(n.Address), escape(n.Interface), value(n.Active), value(n.Installed))
				}
			}
			for _, x := range f.Counters {
				fmt.Fprintf(&b, "  %s: state=%s MTU=%s bytes (layer=%s) speed=%s bit/s; cumulative RX/TX errors=%s/%s drops=%s/%s\n", escape(x.Interface), escape(x.State), value(x.MTUBytes), escape(knownLayer(x.MTULayer)), value(x.SpeedBPS), value(x.RXErrors), value(x.TXErrors), value(x.RXDrops), value(x.TXDrops))
			}
			for _, n := range f.Neighbors {
				fmt.Fprintf(&b, "  LLDP %s: %s port %s\n", escape(n.Interface), escape(n.Name), escape(n.PortID))
			}
			for _, o := range f.Optics {
				fmt.Fprintf(&b, "  DOM %s lane %s: temperature=%s °C RX=%s dBm TX=%s dBm\n", escape(o.Interface), escape(o.Lane), report.Number(o.TemperatureCelsius), report.Number(o.RXPowerDBM), report.Number(o.TXPowerDBM))
			}
			if f.Version != nil {
				fmt.Fprintf(&b, "  OS: %s %s\n", escape(f.Version.OS), escape(f.Version.Version))
			}
			if f.UptimeSeconds != nil {
				fmt.Fprintf(&b, "  Uptime: %s seconds\n", report.Number(f.UptimeSeconds))
			}
			if f.CPU != nil && f.CPU.Utilization != nil {
				fmt.Fprintf(&b, "  CPU reported utilization (window unknown): %s%%\n", report.Number(f.CPU.Utilization))
			}
			if f.CPU != nil && (f.CPU.Utilization1 != nil || f.CPU.Utilization5 != nil || f.CPU.Utilization15 != nil) {
				fmt.Fprintf(&b, "  CPU utilization (1/5/15m): %s%% / %s%% / %s%%\n", report.Number(f.CPU.Utilization1), report.Number(f.CPU.Utilization5), report.Number(f.CPU.Utilization15))
			}
			if f.CPU != nil {
				fmt.Fprintf(&b, "  Load average (1/5/15m): %s / %s / %s\n", report.Number(f.CPU.Load1), report.Number(f.CPU.Load5), report.Number(f.CPU.Load15))
			}
			for _, n := range f.Notes {
				fmt.Fprintf(&b, "  Note: %s\n", escape(n))
			}
		}
	}
	b.WriteString("\nFindings\n")
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "- %s %s: %s [%s]\n  Evidence: %s\n", escape(f.Severity), escape(f.Device), escape(f.Text), escape(f.Rule), escape(strings.Join(f.Evidence, ", ")))
	}
	if len(r.Findings) == 0 {
		b.WriteString("No findings from the available device observations.\n")
	}
	b.WriteString("Device snapshots and the saved trace have different observation windows; they do not establish the earlier forwarding path or a fault. Complete describes requested command coverage, not network health.\n")
	_, err = io.WriteString(w, b.String())
	return err
}
func value[T bool | uint64](v *T) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprint(*v)
}

func contextValue(v *string) string {
	if v == nil {
		return "unknown"
	}
	return *v
}

func knownLayer(layer string) string {
	if layer == "" {
		return "unknown"
	}
	return layer
}
