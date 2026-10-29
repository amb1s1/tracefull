package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/cmd/tracefull/commands/enrich"
)

const saved = "traceroute to dst (192.0.2.2), 30 hops max\n1 192.0.2.1 1 ms !X\n2 192.0.2.2 2 ms\n"

func execute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	c := New()
	var out, errout bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errout)
	c.SetIn(strings.NewReader(saved))
	c.SetArgs(args)
	e := c.ExecuteContext(context.Background())
	return out.String(), errout.String(), e
}
func TestCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{{"enrich", "--from", "-", "--json"}, {"enrich", "--from", "-", "--markdown"}, {"enrich", "--from", "-"}, {"schema"}, {"explain"}, {"version"}, {"--help"}} {
		out, errout, e := execute(t, args...)
		if e != nil || errout != "" || out == "" {
			t.Fatalf("%v: %v %s", args, e, errout)
		}
		if strings.Contains(out, "\x1b") {
			t.Fatal("ANSI in pipe")
		}
		if args[len(args)-1] == "--json" || args[0] == "schema" {
			if !json.Valid([]byte(out)) {
				t.Fatal("invalid JSON")
			}
		}
	}
}
func TestErrorsAndExitFinding(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{{"enrich"}, {"enrich", "--from", "-", "--format", "bad"}, {"enrich", "--from", "-", "--json", "--markdown"}, {"enrich", "--from", "-", "--fail-on", "typo"}, {"trace", "192.0.2.1"}, {"enrich", "--from", "-", "--ssh"}} {
		out, _, e := execute(t, args...)
		if e == nil || out != "" {
			t.Fatalf("%v: %v", args, e)
		}
	}
	out, _, e := execute(t, "enrich", "--from", "-", "--json", "--fail-on", "icmp-error")
	if !errors.Is(e, enrich.ErrFinding) || !json.Valid([]byte(out)) {
		t.Fatal("finding exit must preserve report")
	}
}
func TestEnvironmentAndFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TRACEFULL_FORMAT", "json")
	out, _, e := execute(t, "enrich", "--from", "-")
	if e != nil || !json.Valid([]byte(out)) {
		t.Fatal("environment ignored")
	}
	out, _, e = execute(t, "enrich", "--from", "-", "--format", "markdown")
	if e != nil || !strings.HasPrefix(out, "# tracefull") {
		t.Fatal("flag did not override env")
	}
}
func TestInventoryCheck(t *testing.T) {
	p := filepath.Join(t.TempDir(), "inventory.json")
	if e := os.WriteFile(p, []byte(`{"devices":[]}`), 0600); e != nil {
		t.Fatal(e)
	}
	out, _, e := execute(t, "inventory", "check", p)
	if e != nil || !strings.Contains(out, "0 devices") {
		t.Fatal(e)
	}
}

func TestFormatAliasOverrideAndLogs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TRACEFULL_JSON", "true")
	out, _, e := execute(t, "enrich", "--from", "-", "--format", "markdown")
	if e != nil || !strings.HasPrefix(out, "# tracefull") {
		t.Fatal("explicit format must override inherited shorthand")
	}
	out, logs, e := execute(t, "enrich", "--from", "-", "--json", "-v", "--log-format", "json")
	if e != nil || !json.Valid([]byte(out)) || !json.Valid([]byte(logs)) {
		t.Fatalf("logs separated: %v %s", e, logs)
	}
	_, logs, e = execute(t, "enrich", "--from", "-", "-v", "--quiet")
	if e != nil || logs != "" {
		t.Fatal("quiet did not suppress logs")
	}
}

func TestCLIShorthandOverridesEnvironmentAlias(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TRACEFULL_JSON", "true")
	out, _, e := execute(t, "enrich", "--from", "-", "--markdown")
	if e != nil || !strings.HasPrefix(out, "# tracefull") {
		t.Fatalf("alias precedence: %v", e)
	}
}
