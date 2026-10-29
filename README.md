# tracefull

tracefull interprets saved traces, matches hop addresses to inventory devices,
and reports what the observations support. v0.1 reads files; it does not send
probes, open SSH sessions, or collect device output.

## Build and try it

Requires Go 1.27.3. From this checkout:

```sh
go build -o bin/tracefull ./cmd/tracefull
./bin/tracefull enrich --from testdata/dublin-upstream.json
./bin/tracefull enrich --from testdata/dublin-upstream.json --json
./bin/tracefull enrich --from testdata/dublin-upstream.json --markdown
```

The example is a scrubbed upstream integration fixture, not a new measurement.
Its [provenance and license](testdata/README.md) travel with the file. No DNS runs
unless enabled. The binary can be installed from the checkout with
`go install ./cmd/tracefull`; no release has been published.

## Commands

| Command | Behavior |
| --- | --- |
| `enrich --from FILE` | Parse, enrich, interpret and render a saved trace. Use `-` for stdin. |
| `inventory check FILE` | Validate YAML/JSON; report duplicate addresses and unknown platforms. |
| `explain` | Show implemented rules with examples; Glamour presentation on a terminal. |
| `schema` | Print the embedded report JSON schema, version `"1"`. |
| `version` / `--version` | Show the development version. |
| `completion` / `man` | Generate shell completion or manpage output through Cobra/Fang. |

`trace` and `collect` belong to later milestones and are not available.
Inputs and inventories are limited to 16 MiB. Input paths must be regular files;
use stdin for streams. Raw captures remain in their original files. Reports
contain a SHA-256 digest and line/JSON-pointer references into that input.

## Input formats and limits

| Dialect | Supported representation | Interpretation limits |
| --- | --- | --- |
| Linux/BSD/macOS traceroute | Numeric or hostname/IP rows, IPv4/IPv6, multiple responders (including wrapped rows), stars, annotations; headerless rows with `--dest` | EOF is not evidence of a completed run. Unparsed extension lines produce notes. Save header/stderr with the capture. |
| Windows tracert | English text, numeric or hostname/IP rows, timeout rows, `<1 ms` | Censored RTTs stay unknown; localized headings are unsupported. |
| tracepath | PMTU, retries, `asymm`, `reached`, `Resume`, no-reply rows | Retry statistics are not combined; asymmetry is an imported hint. |
| MTR | Report/wide/both-address/ASN text (`-r`, `-rw`, `-rwb`, `-rwz`) and upstream `--json` | Text normally lacks destination metadata: supply `--dest IP`. Hostname-only identity stays unresolved without a numeric address. Zero sent probes do not become zero loss. |
| Paris traceroute | Hop-by-hop text and exhaustive min/avg/max/stddev rows | Aggregate output does not reconstruct individual flows or sample counts. MPLS/other extension lines remain source notes. |
| Dublin traceroute | Upstream `flows` JSON with sent/received packet records | Preserve separate flow IDs, UDP ports, available timestamps and NAT-change hints. Summary-hop statistics stay null rather than mixing flows. |

Unknown formats, malformed JSON, inconsistent RTT statistics, duplicate TTLs and
invalid counts fail. Partial text input retains usable hops with warnings and
unknown completion. `--dest` must be an IP and cannot contradict a numeric
saved destination. Forward hostname resolution is not performed.

## Inventory and DNS

```yaml
devices:
  - name: lab-leaf
    address: 192.0.2.10
    platform: nvidia-cumulus
    role: leaf
    site: lab
    notes: "Documentation example; no device connection"
    addresses: [192.0.2.1/31]
```

Match order: `--map IP=DEVICE`, interface addresses, management address, exact
rDNS name, then longest dash/underscore-delimited device-name prefix. A prefix
such as `192.0.2.1/31` matches **only 192.0.2.1**. Ambiguous matches list candidates
and leave the chosen device empty. Imported names are eligible only when the hop
has one numeric responder; live PTR results remain separate in `rdns`.

Default inventory discovery checks `./inventory/devices.yaml`, then
`$XDG_CONFIG_HOME/tracefull/devices.yaml` (fallback `~/.config`). Missing defaults
produce a report note. An explicitly selected missing/invalid inventory fails.

`--rdns` opts into PTR queries. `--asn` opts into
[Team Cymru origin-AS DNS queries](https://www.team-cymru.com/ip-asn-mapping)
for public addresses; private, loopback, link-local and documentation addresses
are excluded. Each query has a `--rdns-timeout` deadline (default 1s). Results are
cached only within the run. Lookup failures preserve the report with notes.
DNS observation time is separate from imported measurement time. Tests use fakes;
external DNS availability is not verified by the test suite.

## Configuration and output

Precedence is **defaults < user config < project config < environment < flags**.
User config: `$XDG_CONFIG_HOME/tracefull/config.yaml`, falling back to
`~/.config/tracefull/config.yaml`. Project config: `./tracefull.yaml`.

```yaml
format: json
rdns: false
asn: false
rdns_timeout: 1s
latency_jump: 15ms
fail_on: [real-loss, dest-only-loss]
```

Keys mirror `enrich` flags; underscores or hyphens are accepted. Environment
names use `TRACEFULL_`, uppercase and underscores, e.g. `TRACEFULL_FORMAT=json`.
Lists use YAML arrays or CSV environment values. CLI lists replace inherited
lists; repeated CLI `--map` values accumulate. Unknown config keys fail.
`--no-rdns` disables inherited `rdns`. `--json` and `--markdown` are exclusive
format aliases. Aliases are normalized within each configuration layer before
merging; contradictory values in one layer fail. Command-line format choices
override inherited aliases.
Configuration applies to `enrich`; inspection commands have their own flags.

Text/Markdown measurements use at most three fractional digits; JSON retains
full precision. Text/Markdown show duplicate address/name values once, compact
fully unmatched identity rows, and print limitations shared by every finding once.
Mixed inventory matches retain each address's identity or ambiguity; JSON retains
all original fields and per-finding limitations.
Reports go to stdout, diagnostics to stderr. `enrich -v` adds diagnostics;
`--log-format json` makes those machine-readable and `--quiet` suppresses them.
Errors remain visible. `--no-color`, `NO_COLOR`, non-TTY output and `TERM=dumb`
disable report styling. JSON and Markdown always contain source text without
ANSI. Untrusted terminal controls and Markdown are escaped in views; JSON retains
original strings with JSON escaping. There is no telemetry.

| Exit | Meaning |
| --- | --- |
| 0 | Report/inspection completed; findings are allowed by default. |
| 1 | A requested `--fail-on` finding was present; the report was still written. |
| 2 | Usage, input, inventory, configuration or output failure. |
| 130 / 143 | Interrupted / terminated. An incomplete input produces no report. |

`--fail-on` accepts implemented finding kinds, for example
`--fail-on real-loss,dest-only-loss,unreachable,mtu-drop`. Unknown kinds fail.
Collection/probing exit 3 is reserved for later milestones. Pager/live-view,
SSH and probing flags are unavailable in v0.1.

## What a finding means

Loss classification needs a known responding destination, contiguous comparable
hops and at least 100 sent probes at each compared hop. Imported MTR `Snt` supplies
that count; `cycles` alone does not. This is a minimum sample floor, not a
statistical confidence guarantee. Multiple responders suppress aggregate loss
comparisons; Dublin flows are assessed separately.

The display label `multiple responders / paths` describes different responding
addresses at one TTL or differences between recorded flow paths. A TTL is a
probe position, not one identified router. This observation alone establishes
neither ECMP nor a network fault. The JSON kind `ecmp`, rule `v0.1/ecmp`, severity
`warn` and `--fail-on ecmp` remain stable; `warn` calls attention to interpretation
limits. A multiple-responder finding at one TTL references only that hop's source
evidence.
When comparing saved flows, the finding points to the first TTL with differing
known responding address sets and cites those flow observations. Missing replies
or different capture lengths alone do not establish path variation. This TTL
does not locate the forwarding decision that produced the difference.

`phantom-loss` means response loss decreases downstream; it does not prove CoPP.
`real-loss` is a persistent response-loss pattern, not proof of a faulty link.
At the overlapping 50% reduction/retention boundary, `phantom-loss` takes
precedence for that hop. Destination-only loss is a separate finding. Hop-limit
takes precedence over unreachable; truncated captures keep reachability unknown.
RTT includes reply generation and the return path. `explain` has every rule and
[the schema contract](docs/schema.md) describes attribution and unknown values.

## Development and verification

```sh
make tools                 # install pinned checking tools into .tools/
make check                 # tests, gofmt, vet, race, coverage, lint, docs, build
make fuzz                  # 30-second offline parser mutation run
make scan                  # govulncheck; needs vulnerability database access
make cross                 # compile six OS/architecture pairs; does not run them
```

Python 3 and Make are used by development checks. Routine Go tests are offline
once dependencies are downloaded. Schema and executable integration tests run
inside `go test ./...`. Coverage gates are independently 90% for parser packages
and verdicts; there are no v0.1 drivers.

Focused examples: `go test ./internal/verdict -v` and
`go test ./cmd/tracefull -run TestExecutable -v`. To update report expectations,
review the behavior first, then run
`TRACEFULL_UPDATE_GOLDENS=1 go test ./internal/report -run TestGolden` and inspect
the diff. Parser snapshots use the same opt-in variable with
`go test ./internal/importer -run TestParserGoldens`; their inputs are labeled
synthetic cases in the test file. Ordinary tests never regenerate expectations.

Runtime verification so far is macOS arm64 only. Cross-compilation and a configured
CI matrix do not establish Linux/Windows runtime support. Windows console
interrupt handling is not locally verified. No live devices, external probe
traffic, release artifacts or production behavior were tested.

Layout: `cmd/tracefull/commands` contains CLI adapters; `internal/app` composes the
pipeline; `internal/parse` contains pure dialects; `inventory`, `enrich`, `verdict`,
`report`, `schema`, `config`, and `ui` own their named responsibilities. Registration
is explicit; there are no runtime plugins or `init()` registrations.

See [contributing](CONTRIBUTING.md), [dependency choices](docs/dependencies.md),
and [current verification state](context/current-feature.md). MIT licensed;
third-party fixture notices remain alongside their data.
