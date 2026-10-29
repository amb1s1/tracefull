# tracefull — specification

**tracefull** is a traceroute that tells you the full story of each hop: not just an
interface address and a control-plane RTT, but which device it is, what the numbers prove,
and — when you have read-only access — what the device itself says about this path.

Status: draft v0.1 · Language: Go · License: MIT · Module path:
`github.com/amb1s1/tracefull` (confirmed)

**Implementation status:** v0.1 implementation authorized; see current-feature.md
for verified progress. Later milestones below remain intended product behavior.
The first implementation milestone is **v0.1 — enrich** (§13).

Development guidance: [repository instructions](../AGENTS.md),
[coding standards](coding-standard.md), and
[AI interaction guide](ai-interaction.md).

---

## 1. Problem

Every network engineer reads traceroute, and most of the time they read it wrong. The
output invites three mistakes that this tool is built to remove:

1. **Phantom loss.** A hop that drops 60% of probes while the destination drops none is a
   router rate-limiting ICMP on its CPU, not a fault. It gets escalated anyway.
2. **The hop is not the device.** `10.1.1.1` is an interface, usually the ingress one, on a
   device the inventory knows under a management address. Mapping the two by hand is slow
   and error-prone, and the egress interface — where the problem usually is — is invisible.
3. **Half the RTT is a return path you cannot see.** Nothing in the output says whether
   the way back goes through the same hops, and most tools never ask the devices.

Engineers close this gap by hand: trace, grep inventory, SSH into each device, run four
show commands, paste into a ticket. tracefull does exactly that loop, reproducibly, with
the interpretation rules written down and the evidence attributed to the command that
produced it.

## 2. Goals and non-goals

### Goals

- **One command from symptom to attributed evidence.** Trace, enrich, verdict, device
  facts, markdown you can paste into a ticket.
- **Correct by default.** The verdict engine encodes the interpretation rules (§6) so that
  "phantom vs real loss" is computed, not eyeballed. The tool should never print a number
  without saying what it does and does not prove.
- **Flow-faithful.** Trace the victim flow's 5-tuple (TCP/UDP port, DSCP) with a constant
  flow identifier, so the printed path is the path that flow takes under ECMP.
- **Read-only, provably.** Device commands come from a fixed allowlist per platform. A
  `--dry-run` prints every command before anything connects. There is no code path that
  sends a configuration command.
- **Evidence discipline.** Every raw output is stored with host, command, timestamp and
  exit status, and every parsed fact points at its raw file.
- **Bring your own access.** Native SSH (agent, keys, jump hosts) *or* any wrapper
  command (`netsh`, `nssh`, `tsh ssh`, …) via a template. No credentials on the command
  line, ever.
- **Single static binary**, Linux/macOS/Windows (device collection and parsing everywhere;
  native probing where the OS allows).

### Non-goals (v1)

- Not a monitoring daemon or continuous MTR replacement. The live view (§10.5) shows one
  run as it happens; it is not a long-running dashboard.
- Not a configuration tool. No push, no remediation, no "fix it" button.
- Not a NetBox/IPAM client beyond reading an exported inventory file (a fetcher can come
  later as a separate command, §13).
- Not a packet-capture tool. When counters can't answer, tracefull says so and hands off.
- Not a replacement for vendor-structured APIs (gNMI, NETCONF, EOS eAPI). v1 drives the CLI
  because that is what read-only accounts and jump hosts universally allow. Structured
  transports are a later driver option, not a prerequisite.

## 3. Users and primary stories

| Who | Story |
|---|---|
| On-call network engineer | "Storage VIP has 8% loss from pod 3. Which device, which interface, since when, and is the return path the same?" — one command, markdown into the incident channel. |
| NOC / tier-1 | "MTR shows 60% at hop 3, open a P1?" — tracefull says *phantom* with the rule that proves it, no escalation. |
| SRE / platform engineer without device access | Trace + enrich + verdict with no SSH at all; the output names the devices and the exact read-only commands a network engineer would run next (`--dry-run`). |
| AI agent / automation | JSON output with a stable schema, non-zero exit on real findings, dry-run as a PRE block, evidence directory as the audit trail. |
| Network engineer on a vendor-mixed fabric | Cumulus leaves, NX-OS spines, Junos edge: one inventory, per-platform command sets, one report. |

## 4. CLI surface

Single binary, subcommands. All flags also settable in a config file (§11).

```
tracefull trace   <dest> [flags]       # run a trace, enrich, verdict, optionally collect
tracefull enrich  --from FILE|- [flags] # same pipeline on saved traceroute/mtr/tracepath output
tracefull collect --from FILE|- [flags] # device collection only, from a saved trace (implies --ssh)
tracefull explain                       # print the verdict rules (what each finding kind means)
tracefull platforms                     # list supported platforms and their command sets
tracefull version
```

`trace` is the default when the first argument is not a subcommand (`tracefull 10.50.2.10`).

### 4.1 Trace flags

| Flag | Meaning |
|---|---|
| `--tcp PORT` / `--udp PORT` / `--icmp` | Probe type. `--tcp` and `--udp` keep the 5-tuple constant per path (paris-style) so one run = one real path. Default: ICMP where permitted, else UDP. |
| `--flows N` | Trace N distinct flows (vary source port) and merge: enumerates ECMP paths. Default 1. |
| `--dscp N` / `--tos N` | Mark probes like the application's traffic. |
| `--source ADDR` / `--iface NAME` | Source address or interface. |
| `--max-hops N` (30), `--first-hop N` (1), `--probes N` (3), `--wait DUR` (2s), `--interval DUR` (0) | Classic knobs. |
| `--cycles N` | Repeat the whole trace N times for per-hop loss statistics (mtr-style; default 1 for `trace`, use ≥100 before trusting loss %). |
| `--mtu` | Path-MTU discovery mode (DF set, step down on frag-needed; reports per-hop PMTU like tracepath). |
| `--external mtr|traceroute|tracepath` | Shell out to the system tool instead of the native prober (for environments where raw sockets are unavailable), parse its output. |
| `-4` / `-6` | Address family. |

### 4.2 Enrichment flags

| Flag | Meaning |
|---|---|
| `--inventory PATH` | devices.yaml / .json (§8). Default: `./inventory/devices.yaml`, then `$XDG_CONFIG_HOME/tracefull/devices.yaml`. |
| `--map IP=DEVICE` | Force a hop address onto an inventory device (repeatable). |
| `--no-rdns`, `--rdns-timeout DUR` | Reverse DNS control. |
| `--asn` | Origin ASN per public hop (Team Cymru DNS; needs outbound DNS). |
| `--latency-jump DUR` (15ms) | RTT rise between hops that counts as a jump. |
| `--dest ADDR` | Destination for `--from` input without a header. |

### 4.3 Device-collection flags

| Flag | Meaning |
|---|---|
| `--ssh` | Enable collection on every hop that maps to an inventory device. |
| `--ssh-cmd TEMPLATE` | Use a wrapper instead of native SSH. Placeholders: `{host}` `{address}` `{user}` `{jump}` `{platform}` `{cmd}`. Template is split with shell-words rules; `{cmd}` becomes a single argument. |
| `--user U`, `--jump HOST`, `--identity PATH`, `--known-hosts PATH`, `--insecure-host-key` | Native SSH options. Auth order: agent → identity files → nothing (never password prompts in non-interactive mode). |
| `--connect-timeout DUR` (5s), `--cmd-timeout DUR` (25s), `--workers N` (4) | Limits. One device at a time per host; N hosts in parallel. |
| `--collect LIST` | Comma list of groups: `ingress,route,counters,optics,lldp,version,cpu,reverse`, or `all`. Default `ingress,route,counters,lldp,version`. |
| `--extra-cmd CMD` | Additional command per device (repeatable). Refused unless it passes the read-only allowlist. |
| `--owner-sweep`, `--site S`, `--role R` | Find owners of unmapped hop addresses by listing addresses on inventory devices (optionally filtered). |
| `--dry-run` | Print the PRE block (every command, per device, exactly as it would be executed) and exit 0. Connects to nothing. |
| `--evidence-dir DIR` | Write raw trace, every command output, and `report.json`. |

### 4.4 Output flags

| Flag | Meaning |
|---|---|
| `--format text|json|markdown` (`-o`) | Report format. `text` is the default on a TTY. |
| `--json` / `--markdown` | Shorthands. |
| `--live` / `--no-live` | Live view while probing and collecting (§10.5). Default: on when stdout is a TTY and format is `text`; always off for `json`/`markdown`, in CI, or when piped. |
| `--pager` / `--no-pager` | Page the final report (`explain`, markdown previews). |
| `--fail-on real-loss,unreachable,mtu-drop` | Exit 1 when any listed finding kind is present. Default: none (exit 0). |
| `--quiet`, `-v`, `--log-format text|json` | Logging to stderr only; stdout is always the report. |
| `--no-color` | Also honours `NO_COLOR`. |

### 4.5 Exit codes

| Code | Meaning |
|---|---|
| 0 | Completed (findings may exist; see `--fail-on`). |
| 1 | A `--fail-on` finding kind is present. |
| 2 | Usage error, unreadable input, inventory missing when required. |
| 3 | Probing impossible (no raw-socket permission and no external tool), or all device collection failed. |

## 5. Architecture

```
cmd/tracefull/          process lifecycle and exit status; no feature logic
cmd/tracefull/root/     root command; explicit command registration and dispatch
cmd/tracefull/commands/<name>/  one package per subcommand: New() *cobra.Command
internal/trace/         Trace, Hop, Probe types; merge of multi-flow / multi-cycle runs
internal/probe/         native prober: icmp/udp/tcp senders, receiver, flow-id control,
                        PMTU mode; platform build tags (linux, darwin, windows)
internal/parse/         dialect contract, detection, parsing, explicit registry
internal/parse/<dialect>/  traceroute, tracert, tracepath, mtr, paris, dublin
internal/enrich/        rDNS, ASN, address tagging, inventory matching
internal/inventory/     devices.yaml loader + schema validation
internal/verdict/       the rule engine (§6) — pure functions over Trace, table-tested
internal/device/        Collector orchestration, Session interface, evidence writer,
                        read-only allowlist
internal/device/ssh/    native SSH session (x/crypto/ssh), jump hosts, agent, host keys
internal/device/exec/   wrapper-template session (--ssh-cmd)
internal/driver/        Driver interface + registry; one package per platform:
                        iosxe, nxos, eos, junos, cumulus, frr, linux, routeros
internal/report/        text, json (schema-versioned), markdown renderers
internal/ui/            lipgloss theme (one place for every colour/style), live Bubble Tea
                        model, glamour renderer, TTY/colour-profile detection
internal/schema/        JSON schema for report.json (embedded, served by `tracefull schema`)
testdata/               fixtures: tool outputs, device show outputs, inventories
```

Rules:

- Command + Registry with compile-time composition. Each subcommand exposes
  `New() *cobra.Command`; register commands explicitly in one place. Keep feature
  logic in `internal/<feature>/`, never in main/root. Domain packages must not
  import Cobra, terminal libraries, or CLI adapters.
- Register parser dialects explicitly; no `init()`, Go plugins, HashiCorp
  go-plugin, or import-side-effect registration. The registry must not depend
  on concrete dialects that import its contract; assemble them outside it.
- `internal/verdict` and `internal/parse` are deterministic and perform no I/O;
  standard-library and shared domain types are allowed. They carry the test weight.
- `internal/driver/<platform>` owns *both* the command set and the parsers for that
  platform, so a vendor quirk is fixed in one place. Every driver has golden-file tests
  against real-world outputs in `testdata/`.
- `internal/device` never knows what a command means; it only knows the allowlist, the
  session, the timeout and where to write the evidence.
- The report is rendered from one `Report` struct. JSON is the source of truth; text and
  markdown are views of it. Nothing is computed in a renderer.

### 5.1 Dependencies (keep the list short)

| Need | Choice | Why |
|---|---|---|
| CLI | `github.com/spf13/cobra` | Subcommands, completion, help; de facto standard. |
| SSH | `golang.org/x/crypto/ssh` (+ `/agent`, `knownhosts`) | Stdlib-adjacent, no cgo. |
| Raw sockets | `golang.org/x/net/icmp`, `ipv4`, `ipv6` | Native prober. |
| YAML | `gopkg.in/yaml.v3` | Inventory and config. |
| Testing | stdlib `testing` + `github.com/google/go-cmp` | Golden files and struct diffs. |
| Styling | `github.com/charmbracelet/lipgloss` | Every colour, border and table in the text report comes from one theme. |
| Live view | `github.com/charmbracelet/bubbletea` + `bubbles` (spinner, progress, table, viewport) | Hops appearing as they answer, device collection progress per host. |
| Markdown | `github.com/charmbracelet/glamour` | Renders `explain`, docs and `--format markdown` previews in the terminal. |
| Logging | `github.com/charmbracelet/log` | Styled, levelled stderr logs; `slog` handler so libraries stay on stdlib. |
| CLI polish | `github.com/charmbracelet/fang` | Cobra wrapper: styled help/usage/errors, man pages, completions, `--version`, consistent across subcommands. |

No ORM or runtime plugin loader. Cobra and Charm libraries stay in `cmd/` and
`internal/ui`; domain logic remains independent of CLI and terminal frameworks.

## 6. Verdict engine

The engine applies these rules, in this order, to a `Trace`. Every finding has a `kind`,
a `hop`, a `severity` (`info` / `warn` / `problem`), a one-sentence `text`, and a `rule`
reference. `tracefull explain` prints this table.

| Kind | Rule | Severity |
|---|---|---|
| `unreachable` | Destination never answered; last responder is hop N. Fault at or after N, or the destination filters probes — recommend a TCP probe to the real service port. | problem |
| `hop-limit` | Trace stopped at `--max-hops` while hops were still answering. Not a reachability verdict. | warn |
| `phantom-loss` | Hop N shows loss but a later hop (and the destination) shows ≥50% less. ICMP rate-limiting / CoPP on N's control plane; ignore. | info |
| `silent-hop` | Hop N never answers but later hops do. It forwards; it just doesn't emit Time Exceeded. | info |
| `real-loss` | Loss ≥1% starts at hop N and every later hop including the destination shows ≥50% of that rate. Real loss at/after N — or on the return path from N. | problem |
| `dest-only-loss` | Only the destination shows loss. End-to-end loss on the last segment, or destination rate-limiting; cross-check with a TCP probe. | problem |
| `latency-added` | Avg RTT rises ≥ `--latency-jump` at hop N and every later hop stays ≥ previous + half the jump. Added path latency at/after N (includes the unseen return path). | warn |
| `latency-control-plane` | RTT rises at N but drops back at N+1. Slow ICMP generation on N; ignore. | info |
| `jitter` | Wrst − Best at a hop exceeds max(50 ms, 5× (Avg − Best)). Queueing or control-plane jitter, not distance. | info |
| `ecmp` | One hop answered from ≥2 addresses (or `--flows` found ≥2 distinct paths). Numbers at and after that hop are a braid; pin the flow. | warn |
| `path-mismatch` | Device at hop N routes the destination via next hops that do not include hop N+1's address. ECMP member not in the trace, path change, or reply from another interface of the same device. | warn |
| `mtu-drop` | Measured path MTU decreases at hop N (`--mtu` / tracepath), or a collected egress interface MTU is below the path MTU measured so far. | problem |
| `asymmetry` | tracepath `asymm N` or a collected reverse path whose hop set differs from the forward path. The return path is not the mirror of what you see. | info |
| `icmp-error` | `!H` `!N` `!X` `!P` from hop N. A router explicitly refused the packet; the path ends here. | problem |
| `nat` | (with `--flows`) IP-ID reflection shows a NAT at hop N (dublin-traceroute technique). | info |

Thresholds are constants in one file with the rationale in comments; the only tunable in
v1 is `--latency-jump`. Loss rules require `--cycles ≥ 100` to be trusted and the report
says so when fewer samples were taken.

## 7. Probing

### 7.1 Native prober

- Senders: ICMP echo, UDP (base port 33434), TCP SYN. Receiver: raw ICMP socket for
  Time Exceeded / Unreachable, plus the TCP socket for SYN-ACK/RST.
- **Constant flow identifier** (paris-traceroute technique): for UDP, keep dst port fixed
  and vary the checksum via payload; for ICMP, keep the checksum constant by adjusting
  payload while varying sequence; for TCP, fixed 4-tuple, vary sequence number. The per-hop
  TTL is the only thing that changes between probes on one path.
- `--flows N` varies the source port (UDP/TCP) or ICMP identifier across N flows and
  reports each as a separate path; the report merges them per hop with a `paths` field.
- `--cycles N` repeats the full sweep; loss% and Best/Avg/Wrst/StDev per hop follow mtr's
  definitions exactly so numbers are comparable.
- PMTU mode: send with DF at the maximum size, on ICMP Fragmentation Needed read the
  next-hop MTU and resend; record per-hop PMTU like tracepath.
- Privilege: needs `CAP_NET_RAW` (Linux), root or an admin-granted BPF device (macOS), or
  admin (Windows). When unavailable: fall back to `--external` if a tool is present, else
  exit 3 with the exact capability to grant (`setcap cap_net_raw+ep`). Unprivileged ICMP
  datagram sockets (`ping_group_range`) are supported on Linux for ICMP mode.
- Concurrency: outstanding probes window (`-N` equivalent) default 16; a per-hop send
  interval to be gentle on rate limiters.

### 7.2 External tools and saved output

`internal/parse` recognises, by content: Linux/BSD/macOS traceroute (with and without
`-n`, multi-address hops, `!X` annotations, header on stderr on macOS), Windows tracert,
tracepath (pmtu, asymm, reached, Resume), mtr `-r`/`-rw`/`-rwb`/`-rwz` reports and
`--json`, paris-traceroute and dublin-traceroute JSON. Unknown input is an error, never a
silent empty trace.

## 8. Inventory

The file is the contract; where it comes from is the user's business.

```yaml
defaults:
  ssh_user: netops-ro
  jump_host: bastion.example.net   # optional
devices:
  - name: dc1-leaf-01
    address: 10.0.255.11            # management / reachability address
    platform: nvidia-cumulus        # selects the driver
    role: leaf
    site: dc1
    notes: "WJH enabled"            # free text; printed next to the device, never parsed
    addresses: [10.0.0.1/31]        # optional: known interface addresses, for direct hop matching
```

Platforms: `cisco-ios-xe`, `cisco-nxos`, `arista-eos`, `junos`, `nvidia-cumulus`, `frr`,
`linux`, `mikrotik-routeros` (aliases such as `ios`, `eos`, `cumulus` accepted). Unknown
platform: the device is matched and printed, but only `--extra-cmd` runs on it.

Matching order per hop address: `--map` → `addresses` list → `address` → reverse-DNS name
equals a device name → device name is a dash/underscore-delimited prefix of the reverse-DNS
first label (`leaf-01-et1` → `leaf-01`, longest wins) → `--owner-sweep` → unmatched.
The report always says *how* a hop was matched.

JSON with the same shape is accepted. A `tracefull inventory check` subcommand validates
the file and reports devices with unknown platforms or duplicate addresses.

## 9. Device collection

### 9.1 Driver interface

```go
type Driver interface {
    Platform() string
    Commands(group Group, ctx Params) []string      // ctx: Dst, Src, Iface, HopIP
    ParseAddresses(out string) []AddrEntry           // iface ↔ address
    ParseRoute(out string, dst string) RouteFacts    // next hops, egress ifaces, protocol
    ParseCounters(out string) CounterFacts           // errors/discards/drops, state, mtu, speed
    ParseOptics(out string) OpticsFacts
    ParseLLDP(out string) LLDPFacts
    ParseVersion(out string) VersionFacts
    ParseCPU(out string) CPUFacts
}
```

Each driver ships with `testdata/<platform>/<command>.txt` captured from real devices
(scrubbed) and a golden `.json` of the parsed result. A parser that cannot find a field
returns it empty and the report prints the raw-file pointer; it never guesses.

### 9.2 Collection groups and what they prove

| Group | Question answered | Needs |
|---|---|---|
| `ingress` | Which interface on this device owns the hop address (confirms the inventory match; a miss is flagged as *probably the wrong device*). | — |
| `route` | Egress interface(s) and next-hop set toward the destination; ECMP width; protocol. | — |
| `counters` | Errors, CRC, discards, drops, state, speed, MTU on ingress and every egress member. | `ingress`/`route` |
| `optics` | DOM RX/TX power, temperature on those interfaces. | `ingress`/`route` |
| `lldp` | Physical neighbour and port on the egress interface — is the next device the one the route says? | `route` |
| `version` | OS version and uptime (recent reboot?). | — |
| `cpu` | Control-plane load — explains phantom loss and slow ICMP. | — |
| `reverse` | A traceroute from the device back to the source: the return path. | source address |

Two phases per device: phase 1 (`ingress`, `route`, `version`, `cpu`, `reverse`) in
sequence; phase 2 (`counters`, `optics`, `lldp`) for each interface discovered in phase 1.
Cross-device checks after collection: path-mismatch (§6), interface MTU vs path MTU,
forward vs reverse hop sets.

### 9.3 Sessions

- **Native SSH:** one connection per device, commands multiplexed as separate exec channels
  in sequence. Auth: ssh-agent, then `--identity` / default key files; host keys verified
  against `known_hosts` unless `--insecure-host-key`. `-J`-style jump via a nested
  connection. Platform-specific pre-commands (`terminal length 0`, `set cli screen-length
  0`, `terminal width 511`) are part of the driver, not the session.
- **Wrapper (`--ssh-cmd`):** execute the template per command; stdout is the output,
  non-zero exit is recorded, never retried with different credentials. This is how
  company tooling (TACACS bastions, Teleport, OTP proxies) plugs in without tracefull
  knowing anything about it.
- Per command: timeout, byte cap (1 MiB, truncated with a marker), timing, exit status.
- Failure policy: a failed command is a recorded fact, not a retry loop. At most one
  reconnect per device.

### 9.4 Read-only guarantee

- Every command string — driver-generated or `--extra-cmd` — must match the platform's
  read-only pattern set (prefixes such as `show `, `ip -br`, `ethtool -S`, `/ip route
  print`, …) **and** must not contain a mutating word (`configure`, `clear`, `reload`,
  `write`, `copy`, `delete`, `set `, `commit`, `request`, `nv set`, …) or a shell
  metacharacter (`;` `&` `>` `$(` backticks) outside a driver-owned `| grep/match/include`.
- The check lives in `internal/device/allowlist.go` with table tests that assert every
  driver command passes and a list of known-bad commands fails. CI fails if any driver adds
  a command that does not pass.
- `--dry-run` renders the exact argv per command. A command that never appeared in a
  dry-run cannot be produced by a different code path: dry-run and execution share the
  same planner.

## 10. Output

### 10.1 JSON (`report.json`, schema v1)

Top level: `schema_version`, `tracefull_version`, `started_at`, `dest`, `dest_addr`,
`source_addr`, `probe` (type, port, dscp, flows, cycles, tool), `dest_reached`, `notes[]`,
`hops[]`, `paths[]` (for `--flows`), `findings[]`, `devices[]`, `owner_sweep[]`.

`hops[]`: `ttl`, `addresses[]`, `names{}`, `rdns{}`, `asn{}`, `sent`, `lost`, `loss_pct`,
`best`, `avg`, `worst`, `stdev`, `pmtu`, `tags[]`, `notes[]`, `device{name, platform, role,
site}`, `matched_by`.

`devices[]`: `device`, `platform`, `hop`, `hop_address`, `facts{ingress_iface, route{},
counters{iface: {...}}, optics{}, lldp{}, version{}, cpu{}, reverse_path{}}`, `commands[]`
(`group`, `cmd`, `argv`, `ok`, `exit`, `seconds`, `evidence` path, `error`), `skipped[]`.

The schema is embedded and printed by `tracefull schema`; a breaking change bumps
`schema_version`.

### 10.2 Text

Header (dest, probe, flow, reached), a hop table (`hop  address  loss  avg  best  wrst
pmtu  who`), a *Verdicts* block with severity marks, then *Device evidence* per hop with
one line per fact and the evidence pointer. Colour only on a TTY.

### 10.3 Markdown

Same content as text in a form that pastes cleanly into a ticket or incident channel:
a table, a findings list, a per-device section with each command rendered as
`` `host $ cmd` → status — evidence/path `` so the reader can reproduce it.

### 10.4 Evidence directory

```
<dir>/trace.txt                      raw probe output or native probe log
<dir>/report.json
<dir>/<device>/NN-<group>-<cmd-slug>.txt   header: host, cmd, argv, exit, seconds, started_at
```

### 10.5 Terminal experience (Charm stack)

The terminal is the primary interface, and it should look like a tool from this decade.
Everything below is built on the Charm libraries listed in §5.1 and lives in
`internal/ui`; the plain writers in `internal/report` stay the fallback and the contract.

**Theme.** One `lipgloss` theme: severity colours (`problem` red, `warn` amber, `info`
dim), device/platform badges, muted private-address tags, a hop table with lipgloss
`table` borders, and a findings block with a coloured rule marker. Adaptive colours for
light and dark terminals; `NO_COLOR`, `--no-color`, and non-TTY output drop to plain
ASCII with the same layout so diffs and logs stay readable.

**Live view (`--live`, Bubble Tea).** One model with three stages:

1. *Probing* — a hop row appears the moment a TTL answers (address, RTT so far,
   reverse-DNS filling in asynchronously), a `bubbles/progress` bar for cycles, a
   spinner on the hop currently being probed, `* * *` rows greyed. ECMP braids show the
   extra addresses inline as they are discovered.
2. *Collecting* — one line per device with a spinner → ✓/✗ per command group, elapsed
   time, and the last command's name, so it is obvious which device is slow or refusing.
   `--workers` devices run in parallel; the view shows all of them.
3. *Report* — the final report rendered in a `bubbles/viewport` (scroll, `/` to filter
   findings, `e` to open the evidence directory path, `q` to quit); the same text is
   printed to stdout on exit so a terminal scrollback and a pipe see the same thing.

Keys: `q`/`ctrl+c` abort cleanly (probing stops, open SSH sessions close, evidence
written so far is kept and the report says it was interrupted).

**Rendered markdown (glamour).** `tracefull explain` renders the verdict rules with a
worked example each; `--format markdown` on a TTY previews the rendered page before the
raw markdown goes to stdout only when `--pager` is set, otherwise raw markdown is written
(pipes and agents must get the source, not ANSI).

**Help and errors (fang).** Styled `--help` for every subcommand with grouped flags
(trace / enrichment / device collection / output), examples under each command, man
pages via `tracefull man`, shell completions via `tracefull completion`, and usage
errors rendered in the same theme as the report.

**Logs (charmbracelet/log).** Levelled, timestamped stderr logs; `-v` adds debug with the
probe/SSH internals; `--log-format json` for machines. Logs never go to stdout.

**Rules.** No UI code in `internal/verdict`, `parse`, `driver`, `device`. The live model
consumes the same events the plain writer consumes (an `Event` channel from the
pipeline), so the two cannot drift. Golden tests render with the colour profile forced to
ASCII (`lipgloss.SetColorProfile(termenv.Ascii)`) so fixtures are stable in CI; the live
model is tested with `teatest` snapshots.

## 11. Configuration

`$XDG_CONFIG_HOME/tracefull/config.yaml` (and `./tracefull.yaml`), flags override. Keys
mirror flag names. Typical use: `ssh_cmd: "netsh -a ro {host} {cmd}"`, `inventory:`,
`collect:`, `evidence_dir:`. Environment variables `TRACEFULL_*` for the same keys. No
secrets in config — the tool has no field for a password or token by design.

## 12. Testing

- **Parsers and drivers:** golden files under `testdata/`; a `go test -update` flag
  regenerates goldens. Contributions adding a platform must add fixtures from a real
  device (scrubbed addresses are fine; shape must be real).
- **Verdict engine:** table tests, one per rule, plus the scenario fixtures (phantom at 2
  with real from 4, hop-limit, ECMP braid, MTU drop).
- **Collection:** an in-process SSH server (`x/crypto/ssh` test server) serving canned
  outputs per (host, command), so the full `--ssh` path runs in CI without network. The
  same canned set drives a `fake-device` test binary for the `--ssh-cmd` wrapper path.
- **Native prober:** unit tests for packet construction and flow-id invariants; an
  integration test tagged `//go:build integration` that traces `127.0.0.1` / `::1` when
  raw sockets are available.
- **Allowlist:** every driver command must pass; a corpus of mutating commands must fail.
- **End-to-end:** `tracefull enrich --from testdata/...` snapshots for text, json and
  markdown, rendered with the ASCII colour profile.
- **UI:** `teatest` snapshots of the live model fed by a recorded event stream; a VHS
  tape (`charmbracelet/vhs`) under `docs/` regenerates the README demo gif in CI so the
  demo never goes stale.
- Coverage gate on `internal/verdict`, `internal/parse`, `internal/driver`: 90%.

## 13. Roadmap

| Milestone | Scope |
|---|---|
| **v0.1 — enrich** | `enrich --from`, all parsers, inventory, rDNS/ASN, verdict engine, text/json/markdown, `explain` (glamour), `schema`, lipgloss theme and fang help. No device access, no native probing. Ships the value for people with zero device access. |
| **v0.2 — collect** | Native SSH + wrapper sessions, drivers for cumulus, linux/frr, nxos, junos, eos, ios-xe, routeros; allowlist; dry-run; evidence dir; owner sweep; cross-device checks; live collection view. |
| **v0.3 — trace** | Native prober (ICMP/UDP/TCP, constant flow id, cycles, `--flows`, PMTU mode), external-tool fallback, live probing view. |
| **v1.0** | Stable JSON schema v1, Homebrew tap, Linux packages, docs site, security policy. |
| Later | NetBox inventory fetcher (`tracefull inventory from-netbox`), structured transports (EOS eAPI, Junos NETCONF, gNMI) as driver sessions, IPv6 flow enumeration, a `diff` command comparing two reports (before/after a change), SNMP ifIndex→name fallback for devices without SSH. |

## 14. Open-source readiness checklist

- `LICENSE` (MIT), `README.md` (30-second demo gif, install, three examples,
  what the verdicts mean), `CONTRIBUTING.md` (adding a platform = fixtures + driver +
  golden; adding a rule = table test + `explain` entry), `CODE_OF_CONDUCT.md`,
  `SECURITY.md` (private disclosure address; note that the tool never writes to devices),
  `CHANGELOG.md` (keep-a-changelog).
- CI (GitHub Actions): `go vet`, `staticcheck`, `golangci-lint`, `go test -race`,
  `govulncheck`, build matrix linux/darwin/windows × amd64/arm64, allowlist test as a
  separate required check.
- Release: goreleaser, signed checksums, SBOM, Homebrew tap, `go install` path, Docker
  image with `CAP_NET_RAW` documented.
- Docs: `docs/verdicts.md` (the rules, with worked examples of real vs phantom loss),
  `docs/platforms.md` (per-platform commands, exactly what runs), `docs/access.md`
  (agent, jump hosts, wrappers, read-only accounts), `docs/schema.md`.
- Scrubbing policy for fixtures: RFC 5737 / RFC 3849 addresses, no real hostnames,
  no serials.
- Telemetry: none. Outbound traffic is only the probes, DNS, and SSH the user asked for.

## 15. Open questions

1. **Module path / owner:** `github.com/amb1s1/tracefull` or an org? Decide before the
   first tag; it is in every import path.
2. **Name collision check** for "tracefull" on GitHub, Homebrew, and PyPI/crates (for
   future ports) before publishing.
3. **Default probe type** when running unprivileged on Linux: ICMP via datagram socket
   (needs `ping_group_range`) or UDP via `IP_RECVERR` without raw sockets? Proposal: try
   both, prefer ICMP, and print which was used.
4. **Driver pre-commands** that change terminal paging are harmless but are technically
   not "show" commands. Proposal: explicitly whitelisted per driver as `session-setup`,
   shown in dry-run like everything else.
5. **Windows native probing:** ship it in v0.3 or document `--external tracert` only?
   Proposal: `--external` only until someone needs it.
6. **Structured output from devices** (`| json`, `| display json`) where available gives
   better parsers but a second code path per platform. Proposal: v1 parses text only;
   drivers may opt into JSON per command later behind the same `Parse*` interface.

## 16. Implementation safeguards and decisions to resolve

The specification above is a draft. The following points qualify its guarantees
and identify decisions that need resolution before the affected feature ships.
They do not add scope to v0.1.

### 16.1 Interpretation is evidence, not proof of a cause

The verdict kinds in §6 are stable design names; their explanations must preserve
uncertainty. `phantom-loss` describes intermediate probe-response loss that does
not persist downstream. Rate limiting or control-plane handling is an explanation
to investigate, not a cause established by the trace alone. Likewise, `real-loss`
is a loss pattern consistent with an end-to-end problem, not proof that hop N
dropped transit traffic. Avoid unqualified “ignore” or “fault at N” messages.

A silent hop followed by responding hops is consistent with forwarding without
visible replies in that measurement. RTT includes forward travel, response
generation, and return travel; a jump does not isolate a link's latency. An ICMP
error describes a particular probe response and need not end every flow.

Preserve flow, sample count, and measurement window when comparing hops. Define
how saved mtr sample counts map to the §6 minimum before implementing loss
confidence: imported data may have no `cycles` metadata. One hundred cycles is
the draft threshold, not a guarantee of statistical confidence. Missing samples
and zero loss must remain distinct. State overlap and precedence explicitly for
`unreachable`/`hop-limit` and `real-loss`/`dest-only-loss`, with tests.

A stable probe flow identifier reduces flow-hash variation; it cannot guarantee
one path under every forwarding implementation or a changing network. A reverse
trace measures a new flow from a device, not the exact return path of an earlier
ICMP reply. Route and hop-set comparisons therefore need qualified wording.

### 16.2 Command planning has discovery dependencies

Phase-2 commands depend on interfaces learned during phase 1 (§9.2). A fresh
offline dry-run cannot name those interfaces while also connecting to nothing.
Before v0.2, define a plan representation that distinguishes resolved commands
from deferred commands, and decide whether a fully concrete preview requires
saved discovery evidence. Do not present unresolved templates as exact argv.
Both preview and execution must use the same planner and command validation.

The proposed terminal setup exceptions in §15 must be explicit per driver and
tested. Unknown platforms can be displayed and matched, but the §8 proposal to
run `--extra-cmd` needs an explicit validation policy; no unknown-platform
allowlist means no execution. Broad command prefixes alone are not a read-only
guarantee. Validate parameters and driver-owned pipelines as well as commands.

### 16.3 Contracts to settle during the relevant milestone

- **v0.1 configuration:** define precedence among defaults, user config, project
  config, environment, and flags. Specify missing-inventory behavior when
  enrichment is optional versus required, and opt-in/default DNS behavior.
- **v0.1 report schema:** define units, null/omitted values, per-fact evidence
  references for later collection, and ordering. Reconcile the CLI listing in §4
  with `schema` and `inventory check` described later. Document only implemented
  commands as available in CLI help.
- **v0.2 collection:** define VRF/routing-instance context for route and reverse
  lookups, partial-failure reporting, evidence-directory permissions, and safe
  behavior for existing output files. A single counter reading is cumulative;
  report a rate or an incident time only when supporting samples exist.
- **v0.3 probing:** validate platform-specific socket privileges and fallback
  behavior experimentally before claiming support. Resolve `tracert` support
  against the external-tool flag list and document the tested OS matrix.
- **Release readiness:** confirm owner, license, supported Go version, dependency
  versions, coverage enforcement, and release checks. The static-binary and
  platform goals require build and runtime verification before publication.

## 17. Release acceptance

tracefull remains a single-process CLI. Startup, cancellation, evidence
persistence, and builds must have explicit, testable behavior. Selected engineering
rules live in [coding-standard.md](coding-standard.md); no external guide is
required to apply them.

### 17.1 Acceptance evidence by milestone

These are implementation targets, not completed checks. Each release must state
which checks ran, on which platforms, and what remains unsupported.

| Milestone | Evidence required for its implemented scope |
|---|---|
| v0.1 — enrich | Fixture-based CLI tests cover malformed input, unknown facts, stable JSON, text/Markdown output, stderr separation, and documented exit codes. Parser fuzz seeds and bounded fuzz runs exercise unexpected input. DNS behavior is explicit and deterministic in tests. |
| v0.2 — collect | Fake-session tests demonstrate command validation, dry-run behavior, deadlines, bounded output, partial failure, cancellation, and preserved evidence. Inject evidence-write failures and verify they are reported. Validate platform parsers against scrubbed device captures. |
| v0.3 — trace | Packet tests verify flow identifiers and response correlation; authorized integration tests verify advertised probing modes and privilege fallback on supported systems. Label cross-compilation separately from runtime verification. |
| Published releases | Record the source revision, Go/tool versions, automated check results, supported platform matrix, and known limitations. Produce the release artifacts specified in §14 and smoke-test the packaged CLI. |

### 17.2 Future service scope

gRPC, microservices, Kubernetes, and remote telemetry are outside the current
roadmap. A service proposal must establish the remote API/deployment need and
trust/operational boundaries first; §14's no-telemetry policy remains in force.

If approved later, define deadlines, cancellation, TLS/authentication, health,
and graceful shutdown; compose only required middleware. Version API contracts
and validate Protobuf lint, generation, and compatibility before publication.
These are conditional design considerations, not current dependencies or proof
of production readiness.
