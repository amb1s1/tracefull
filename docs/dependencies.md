# Dependency choices

Go 1.27.3 is the initial supported toolchain. Versions and transitive requirements
are pinned in [go.mod](../go.mod) and [go.sum](../go.sum). Standard-library code owns
addresses, JSON, DNS, clocks, hashing, file I/O and tests.

| Dependency | Purpose and rationale | License |
| --- | --- | --- |
| Cobra 1.10.2 / pflag 1.0.9 | Fresh command/flag state and explicit registration; domain packages remain independent. | Apache-2.0 / BSD-3-Clause |
| Fang 1.0.0 | Styled help, completion, version flag and manpage generation. | MIT |
| Bubble Tea v2.1.1 / Bubbles v2.2.1 | Live collection model, spinner and report viewport; inspected installed APIs and MIT licenses. Reuses the Charm terminal stack instead of implementing raw terminal handling. | MIT |
| Lip Gloss v2.0.6 / Glamour v2.0.1 | Terminal tables/theme and explanation rendering. A coherent v2 stack avoids incompatible ANSI APIs in the older releases. | MIT |
| go.yaml.in/yaml/v3 3.0.5 | Strict known-field inventory decoding; maintained YAML project path. | MIT / Apache-2.0 |
| jsonschema/v6 6.0.3 | Draft 2020-12 validation in tests; no runtime network schema loading. | Apache-2.0 |
| cancelreader 0.2.2 | Interrupt a blocked Unix stdin read; already in the terminal dependency graph. Standard os.Stdin.Close alone failed the macOS process test. Regular files bypass descriptor polling. | MIT |
| golang.org/x/crypto 0.58.0 | Native SSH, agent keys and known-host verification for collection sessions; inspected installed client authentication and connection APIs. | BSD-3-Clause |
| go-shellwords 1.0.16 | Wrapper tokenization before substitution; no transitive dependencies. Parser environment, command and comment expansion are explicitly disabled, with shell syntax rejected before parsing. Avoids a custom quote parser. | MIT |

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

The collection-session scan on 2026-10-29 found no reachable vulnerabilities and
none in imported packages. It reports GO-2026-5932 at module level for the
unmaintained `golang.org/x/crypto/openpgp` package, which tracefull does not import.
That advisory has no fixed version; it does not apply to the SSH packages used here.

Checking tools are pinned in the Makefile: staticcheck v0.8.2, golangci-lint
v2.15.0 and govulncheck v1.9.0. To update them, review release compatibility,
change the explicit version, reinstall and run every local check. Tool installation
uses CGO_ENABLED=0 because checking tools do not need cgo; race testing remains
cgo-enabled. A local SDK linker problem affected the first linter installation,
not the subsequent race test.

Unix stdout uses a duplicated nonblocking descriptor owned by Go's poller so
cancellation can interrupt a full pipe. Cleanup restores shared descriptor flags
and closes the duplicate. Process tests cover this behavior on the local host.
Linux and macOS are the supported runtime targets. Windows-specific fallback code
and transitive dependencies may remain, but do not imply Windows support.

The collection view requires Bubble Tea’s newer Ultraviolet, x/ansi, runewidth
and terminfo versions. Those four transitive upgrades are recorded in go.mod;
existing rendering and terminal tests are rerun with the new graph. Only spinner
and viewport Bubbles packages are imported; no clipboard package is used.
