package collect

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prepare(t *testing.T, change func(*Options)) Prepared {
	t.Helper()
	dir := t.TempDir()
	inv := filepath.Join(dir, "inventory.yaml")
	if err := os.WriteFile(inv, []byte("defaults:\n  ssh_user: operator\ndevices:\n  - name: switch-a\n    address: 2001:db8:2::3\n    platform: nvidia-cumulus\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o := Defaults()
	o.From = "-"
	o.Inventory = inv
	o.EvidenceDir = filepath.Join(dir, "evidence")
	o.Sudo = true
	o.Groups = append(o.Groups, "cpu")
	if change != nil {
		change(&o)
	}
	p, err := Prepare(context.Background(), o, io.NopCloser(strings.NewReader("traceroute to 2001:db8::ffff (2001:db8::ffff), 30 hops max, 60 byte packets\n 1 2001:db8:2::3 1 ms 2 ms 3 ms\n")))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanValidationBeforeExecution(t *testing.T) {
	p := prepare(t, nil)
	for _, mutate := range []func(*Options){
		func(o *Options) { o.Workers = 0 }, func(o *Options) { o.Groups = []string{"typo"} }, func(o *Options) { o.VRF = "tenant" }, func(o *Options) { o.Extras = []string{"sudo reboot"} }, func(o *Options) { o.Site = "lab" }, func(o *Options) { o.Wrapper = "sh -c {cmd}" },
	} {
		o := p.Options
		mutate(&o)
		if _, err := Prepare(context.Background(), o, io.NopCloser(strings.NewReader(string(p.Raw)))); err == nil {
			t.Fatal("invalid plan accepted")
		}
	}
}
