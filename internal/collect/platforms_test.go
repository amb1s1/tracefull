package collect

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amb1s1/tracefull/internal/device"
	"github.com/amb1s1/tracefull/internal/driver"
	"github.com/amb1s1/tracefull/internal/process"
	"github.com/amb1s1/tracefull/internal/trace"
)

type captureSession struct {
	t       *testing.T
	dialect string
	files   map[string]string
	policy  device.Policy
	seen    []string
	closed  bool
}

func (s *captureSession) Run(_ context.Context, cmd string) (device.Result, error) {
	if err := s.policy(cmd); err != nil {
		return device.Result{}, err
	}
	file, ok := s.files[cmd]
	if !ok {
		s.t.Fatalf("unmapped replay command %q", cmd)
	}
	b, err := os.ReadFile(filepath.Join("..", "driver", s.dialect, "testdata", file))
	if err != nil {
		s.t.Fatal(err)
	}
	s.seen = append(s.seen, cmd)
	code := 0
	return device.Result{Command: cmd, Result: process.Result{Status: "ok", ExitCode: &code, Stdout: b}}, nil
}
func (s *captureSession) Close() error { s.closed = true; return nil }

func TestPlatformCapturePipelines(t *testing.T) {
	cases := []struct {
		platform, dialect, address, dest, iface string
		files                                   map[string]string
	}{
		{"arista-eos", "eos", "2001:db8:3::a", "2001:db8:4::f", "Vlan2602", map[string]string{
			"show ip interface | json": "addresses4.json", "show ipv6 interface | json": "addresses6.json", "show ipv6 route 2001:db8:4::f | json": "route6.json", "show version | json": "version-json.json", "show interfaces Vlan2602 | json": "counters-vlan.json", "show lldp neighbors detail | json": "lldp.json",
		}},
		{"junos", "junos", "192.0.2.4", "192.0.2.5", "et-0/0/0", map[string]string{
			"show interfaces terse | display json": "addresses.json", "show route table inet.0 192.0.2.5 detail | display json": "route4-connected.json", "show version | display json": "version-json.json", "show system uptime | display json": "uptime.json", "show chassis routing-engine | display json": "cpu.json", "show interfaces et-0/0/0 extensive | display json": "counters-physical.json", "show interfaces et-0/0/0.0 extensive | display json": "counters-logical.json", "show interfaces diagnostics optics et-0/0/0 | display json": "optics.json", "show lldp neighbors | display json": "lldp.json",
		}},
		{"cisco-ios-xr", "iosxr", "2001:db8:9::23", "2001:db8:9::22", "FourHundredGigE0/0/0/4", map[string]string{
			"show ipv4 interface brief": "addresses4.txt", "show ipv6 interface brief": "addresses6.txt", "show route ipv6 2001:db8:9::22 detail": "route6-connected.txt", "show version": "version.txt", "show processes cpu": "cpu.txt", "show interfaces FourHundredGigE0/0/0/4": "counters.txt", "show controllers optics 0/0/0/4": "optics.txt", "show lldp neighbors detail": "lldp.txt",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			p := prepare(t, nil)
			p.Options.Groups = []string{"ingress", "route", "version", "cpu", "counters", "optics", "lldp"}
			p.Plan.Destination = tc.dest
			d := p.Plan.Devices[0].Device
			d.Platform = tc.platform
			d.Address = tc.address
			dp, err := devicePlan(p, d, []string{tc.address}, false)
			if err != nil {
				t.Fatal(err)
			}
			p.Plan.Devices = []DevicePlan{dp}
			p.Trace.Hops[0].Addresses = []string{tc.address}
			p.Trace.Hops[0].Matches = map[string]trace.Match{tc.address: {Device: d.Name, Candidates: []string{}}}
			s := &captureSession{t: t, dialect: tc.dialect, files: tc.files}
			r, err := Run(context.Background(), p, func(_ context.Context, _ DevicePlan, policy device.Policy) (device.Session, error) {
				s.policy = policy
				return s, nil
			})
			if err != nil || !s.closed {
				t.Fatal(err, s.closed)
			}
			if !slices.Contains(r.Devices[0].Interfaces, tc.iface) {
				t.Fatal(r.Devices[0].Interfaces)
			}
			optical := 0
			for _, c := range r.Devices[0].Commands {
				if c.Status != "ok" && c.Status != "unsupported" {
					t.Fatal(c.Name, c.Status, c.Reason)
				}
				if c.Facts != nil {
					optical += len(c.Facts.Optics)
				}
			}
			if tc.dialect != "eos" && optical != 4 {
				t.Fatal("missing real DOM lanes", optical)
			}
			for _, f := range r.Findings {
				if f.Kind == "owner-not-observed" {
					t.Fatal("address families were evaluated separately", f)
				}
			}
			validateReport(t, r)
		})
	}
}

func TestPlatformRegistry(t *testing.T) {
	for _, p := range []string{"junos", "arista-eos", "eos", "cisco-ios-xr", "iosxr"} {
		d, ok := Registry(p)
		if !ok || d.Platform() == "" {
			t.Fatal(p)
		}
		if _, err := d.Plan([]string{"route"}, driver.Params{Destination: "192.0.2.1", VRF: "tenant"}); err == nil {
			t.Fatal("VRF fallback")
		}
	}
	for _, p := range []string{"cisco-ios-xe", "cisco-nxos", "frr", "linux", "routeros"} {
		if _, ok := Registry(p); ok {
			t.Fatal("unvalidated driver", p)
		}
	}
	// Commands selected by new drivers must survive generic wrapper validation,
	// including the fixed remote JSON display suffix, without a local shell.
	for _, p := range []string{"junos", "eos", "iosxr"} {
		d, _ := Registry(p)
		params := driver.Params{Destination: "2001:db8::1"}
		cmds, err := d.Plan([]string{"version", "ingress", "route"}, params)
		if err != nil {
			t.Fatal(err)
		}
		w, err := device.NewWrapper("/usr/bin/true {cmd}", device.Target{}, func(cmd string) error { return d.Validate(cmd, params) }, Defaults().CommandTimeout)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cmds {
			argv, e := w.Plan(c.Text)
			if e != nil || argv[len(argv)-1] != c.Text {
				t.Fatal(c, e)
			}
			if strings.Contains(c.Text, "sudo") {
				t.Fatal("unexpected privilege")
			}
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
