# Local tool investigation

`tracefull investigate TARGET` executes installed tools and produces one report.
Each tool's measurements, findings and raw output remain separately attributed.
This is active probing through existing executables; it does not use SSH or
implement native probes.

Start with a preview:

```sh
tracefull investigate 1.1.1.1 --dry-run
tracefull investigate example.com --dry-run --json
```

Dry-run makes no DNS queries, executes no programs and writes no evidence. For an
IP it displays absolute executable paths and argv arrays. For a hostname it
marks executable/argv selection deferred until the address family is known.

Execute only against a target you intend to probe:

```sh
tracefull investigate 1.1.1.1
tracefull investigate 1.1.1.1 --tools traceroute,mtr --json --evidence-dir ./run-001
tracefull investigate 2001:db8::1 --dry-run
```

The first two commands send traffic. A hostname is resolved once, with a
10-second DNS limit; one address is selected deterministically, preferring IPv4,
and passed numerically to every tool. Use an IPv6 literal to select IPv6. Scoped
IPv6 addresses and non-ASCII hostname labels are unsupported. Reverse DNS and
ASN lookups remain opt-in through `--rdns` and `--asn`.

## Tools and limits

| Adapter | Execution |
| --- | --- |
| traceroute | Linux traceroute or macOS traceroute/traceroute6; numeric UDP probes, three queries per TTL, one-second reply wait. |
| mtr | Numeric ICMP JSON report, default 100 cycles. |
| tracepath | Linux only; numeric UDP trace with the tool's PMTU discovery. |

The registry is explicit and accepts only these names. Defaults select all three;
unavailable or unsupported tools are listed in the report. `--tools` selects a
comma-separated subset. Permission errors are reported; tracefull never runs
sudo, changes privileges, installs tools, or substitutes arbitrary commands.
Paris and Dublin remain saved-input parsers without execution adapters.

Defaults are 30 hops, 100 MTR cycles, two minutes per tool and five minutes overall.
Tools run sequentially. `--max-hops` accepts 1–255; `--cycles` accepts 1–1000.
`--tool-timeout` must not exceed `--timeout`, which is capped at 30 minutes.
DNS enrichment is also bounded by the overall deadline. Investigation options
come from flags; the existing configuration files/environment apply to `enrich`.

Each stdout/stderr stream is capped at 1 MiB. Exceeding the cap terminates the
process group and marks the output truncated. Cancellation and deadlines also
terminate the group; inherited output pipes have a one-second cleanup limit.
Tool locale is fixed to C. MTR_OPTIONS/MTR_PACKET overrides are not inherited.

## Evidence and reports

The default creates a unique `.tracefull-evidence-*` directory in the current
directory. The leading dot keeps private evidence out of Go package discovery
when running from a source checkout. The report prints its absolute path.
`--evidence-dir` requires a new directory; an existing directory fails
before any tool runs. Directories use mode 0700 and files 0600.

Running the entire CLI with sudo creates root-owned evidence. To inspect it,
use the corresponding account; tracefull does not change ownership or widen
permissions. Earlier non-hidden evidence directories can prevent `go test ./...`
from traversing a checkout if the current account cannot read them. Use a clean
checkout for development checks in that case.

Every executed tool produces separate stdout/stderr files. Successful output
also gets a parser-input file: traceroute stderr (which carries its header), a
newline, then stdout; MTR/tracepath use stdout. The report records paths, byte
counts, SHA-256 digests, argv, execution timestamps and exit status. Hop evidence
references the parser input's lines or JSON pointers. All files are created
exclusively; existing files are never overwritten. Write failures are fatal and
reported, with already-written evidence retained.

`report.json` is finalized even after a tool failure or cancellation when the
filesystem permits it. On interruption, inspect that file; stdout may be closed.
Execution timestamps are distinct from any measurement times imported from tools.
Loss/RTT samples are never combined across tools. Failed, truncated or timed-out
output is retained but not interpreted as a complete measurement.

Text/Markdown include tool coverage followed by separate tool reports. JSON uses
a versioned investigation envelope containing the existing saved-report objects.
Export both schemas alongside each other for validation:

```sh
tracefull schema > report-v1.json
tracefull schema --kind investigation > investigation-v1.json
```

The envelope references `report-v1.json`. It records `complete`, `partial`,
`failed` or `interrupted`; each tool additionally distinguishes unavailable,
unsupported, timeout, cancellation, truncation and parse failures.

Exit 0 means at least one tool produced a usable report; partial coverage remains
explicit. Exit 3 means no usable report or an overall deadline expired. Exit 2
means usage/configuration/evidence/output failure. Signals retain exit 130/143.

## Verification limits and adapter references

Development tests use synthetic outputs, fake resolvers/executables and local
processes. They verify lifecycle, evidence, schema and CLI behavior without
probing external targets. Real installed tools have been inspected through help
and manuals, but this feature has no live-network verification yet. Different
tool versions may reject flags or emit unsupported formats; those failures remain
visible instead of triggering silent retries with different probe semantics.

Adapters were checked against the macOS traceroute/traceroute6 manuals, installed
MTR help, the [MTR manual](https://raw.githubusercontent.com/traviscross/mtr/master/man/mtr.8.in)
and [iputils tracepath documentation](https://raw.githubusercontent.com/iputils/iputils/master/doc/tracepath.xml).
