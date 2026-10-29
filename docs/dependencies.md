# Dependency choices

Go 1.27.3 is the initial supported toolchain. Versions and transitive requirements
are pinned in [go.mod](../go.mod) and [go.sum](../go.sum). Standard-library code owns
addresses, JSON, DNS, clocks, hashing, file I/O and tests.

| Dependency | Purpose and rationale | License |
| --- | --- | --- |
| Cobra 1.10.2 / pflag 1.0.9 | Fresh command/flag state and explicit registration; domain packages remain independent. | Apache-2.0 / BSD-3-Clause |
| Fang 1.0.0 | Styled help, completion, version flag and manpage generation. | MIT |
| Lip Gloss v2.0.6 / Glamour v2.0.1 | Terminal tables/theme and explanation rendering. A coherent v2 stack avoids incompatible ANSI APIs in the older releases. | MIT |
| go.yaml.in/yaml/v3 3.0.5 | Strict known-field inventory decoding; maintained YAML project path. | MIT / Apache-2.0 |
| jsonschema/v6 6.0.3 | Draft 2020-12 validation in tests; no runtime network schema loading. | Apache-2.0 |
| cancelreader 0.2.2 | Interrupt a blocked Unix stdin read; already in the terminal dependency graph. Standard os.Stdin.Close alone failed the macOS process test. Regular files bypass descriptor polling. | MIT |

Source/API references used for compatibility checks: installed module source,
[Fang](https://github.com/charmbracelet/fang),
[Glamour](https://github.com/charmbracelet/glamour),
[MTR report serialization](https://github.com/traviscross/mtr/blob/master/ui/report.c),
[Dublin results](https://github.com/insomniacslk/dublin-traceroute/blob/master/go/dublintraceroute/results/results.go),
and [Team Cymru DNS](https://www.team-cymru.com/ip-asn-mapping).
No upstream implementation was copied. The Dublin fixture has its own notice.

Initial scanning found reachable issues in older transitive `x/text` and
`goldmark` versions. The module pins patched versions and a compatible newer
`x/net`; the corrected scan reported no vulnerabilities. Repeat the scan when
updating dependencies; this result is time-specific.

Checking tools are pinned in the Makefile: staticcheck v0.8.2, golangci-lint
v2.15.0 and govulncheck v1.9.0. To update them, review release compatibility,
change the explicit version, reinstall and run every local check. Tool installation
uses CGO_ENABLED=0 because checking tools do not need cgo; race testing remains
cgo-enabled. A local SDK linker problem affected the first linter installation,
not the subsequent race test.

Unix stdout uses a duplicated nonblocking descriptor owned by Go's poller so
cancellation can interrupt a full pipe. Cleanup restores shared descriptor flags
and closes the duplicate. Process tests cover this behavior on the local host;
Windows uses its file-close path and still needs native validation.
