// Package linuxfrr composes the Linux/FRR commands validated on Cumulus Linux.
// Reuse the same iproute2, procfs, FRR and lldpd parsers; OS branding is separate.
package linuxfrr

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/driver/cumulus"
)

type Driver struct{}

func (Driver) Platform() string { return "frr" }
func (d Driver) Plan(groups []string, p driver.Params) ([]driver.Command, error) {
	commands, err := (cumulus.Driver{}).Plan(groups, p)
	if err != nil {
		return nil, err
	}
	if slices.Contains(groups, "version") {
		command := frrVersionCommand(p)
		if err = d.Validate(command, p); err != nil {
			return nil, err
		}
		commands = append(commands, driver.Planned("frr-version", "version", command))
	}
	return commands, nil
}
func frrVersionCommand(p driver.Params) string {
	prefix := ""
	if p.Sudo {
		prefix = "sudo -n "
	}
	return prefix + "vtysh -c 'show version'"
}
func (Driver) Validate(command string, p driver.Params) error {
	if p.VRF != "" && p.VRF != "default" {
		return fmt.Errorf("unsupported Linux/FRR routing context")
	}
	if command == frrVersionCommand(p) {
		return nil
	}
	return (cumulus.Driver{}).Validate(command, p)
}

var frrVersion = regexp.MustCompile(`^FRRouting ([0-9]+\.[0-9]+(?:\.[0-9]+)?(?:[-+._a-zA-Z0-9]*)?) \([^\r\n]*\) on Linux\([^\r\n]+\)\.$`)
var osID = regexp.MustCompile(`^[a-z0-9_-]+$`)

func (Driver) Parse(name string, data []byte) (driver.Facts, error) {
	if name != "version" && name != "frr-version" {
		return (cumulus.Driver{}).Parse(name, data)
	}
	f := driver.Empty()
	if len(data) == 0 || len(data) > 1<<20 {
		return f, fmt.Errorf("empty or oversized Linux/FRR output")
	}
	if name == "frr-version" {
		line, _, _ := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		m := frrVersion.FindStringSubmatch(line)
		if m == nil {
			return f, fmt.Errorf("unrecognized FRRouting version header")
		}
		f.Version = &driver.Version{OS: "FRRouting", Version: m[1], Evidence: "line:1"}
		return f, nil
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if key != "ID" && key != "NAME" && key != "VERSION_ID" {
			continue
		}
		if strings.HasPrefix(value, "\"") {
			var err error
			value, err = strconv.Unquote(value)
			if err != nil {
				return f, err
			}
		} else if strings.HasPrefix(value, "'") {
			return f, fmt.Errorf("unvalidated os-release quoting")
		}
		if strings.ContainsFunc(value, unicode.IsControl) {
			return f, fmt.Errorf("control byte in OS identity")
		}
		fields[key] = value
	}
	if !osID.MatchString(fields["ID"]) || fields["NAME"] == "" {
		return f, fmt.Errorf("missing Linux OS identity")
	}
	f.Version = &driver.Version{OS: fields["NAME"], Version: fields["VERSION_ID"], Evidence: "file"}
	return f, nil
}
