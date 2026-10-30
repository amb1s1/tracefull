# Device collection

`collect --from FILE|-` matches a saved trace to inventory and collects attributed,
read-only device snapshots. The first driver supports Cumulus Linux 5.16 / FRR 10.0
capture formats. v0.2 is **in progress**, not a completed multi-platform milestone.
`investigate` runs local probe tools; it does not automatically open SSH sessions.

## Preview and execute

Start with a local inventory and an offline plan:

```sh
./bin/tracefull collect --from testdata/dublin-upstream.json \
  --inventory docs/collection-inventory.example.yaml --dry-run
./bin/tracefull schema --kind collection
```

The example inventory uses documentation addresses and is for preview only.
For an authorized device, substitute your actual saved trace and inventory:

```sh
./bin/tracefull collect --from trace.txt --inventory inventory.yaml --dry-run
./bin/tracefull collect --from trace.txt --inventory inventory.yaml \
  --user operator --sudo --no-live --json --evidence-dir ./collection-run-001
```

The second command opens SSH and performs the planned reads. `--sudo` explicitly
selects `sudo -n` for FRR and LLDP; it never prompts or falls back after a denial.
Do not run the entire CLI under sudo merely to grant these remote reads.
Default context is `default`. Other `--vrf` values fail without falling back.
The matched address's interface master is reported separately from routing context.

Preview and execution share a planner. Preview performs no DNS, connections,
wrapper execution or evidence writes. Interface-dependent commands are `deferred`.
Owner-sweep follow-up commands are `conditional` on unique ownership. The native
SSH command string is exact; wrappers also show the exact local argv.

## Driver and interpretation

| Group | Cumulus implementation | Limits |
| --- | --- | --- |
| ingress | `ip -j address show` | Exact address ownership, not subnet ownership. |
| route | `vtysh -c 'show ip/ipv6 route ADDRESS json'` | Numeric destination, default context; retains prefix, protocol, selection, FIB state and scoped next-hop interface. IPv6 FRR output is capture-tested; IPv4 command construction is tested but its output variant has no live capture yet. |
| counters | `ip -j -s link show dev INTERFACE` | Owning and active installed egress interfaces. Cumulative errors/drops, state and MTU; speed and CRC-specific counters remain unknown. The captured whole-link JSON validates the parser; per-interface selection is tested offline. |
| lldp | `/usr/sbin/lldpctl -f json` | One raw snapshot, parsed neighbors filtered to discovered interfaces. |
| version | `cat /etc/os-release`, `cat /proc/uptime` | Cumulus version and uptime seconds. |
| cpu | `cat /proc/loadavg` | Load averages, not utilization percentages. |
| optics, reverse | Explicitly unsupported | Require validated formats and, for live reverse probing, separate authorization. |

Default groups are `ingress,route,counters,lldp,version`. `--collect all` includes
unsupported groups, so coverage remains partial. Counter/LLDP requests include
ingress/route discovery. Raw output remains intact when parsed rows are filtered.
Malformed output becomes `parse_failed`; missing measurements remain null.
An extra command must pass the same full-command policy; its raw output is saved
without guessing its parser. Unknown platforms cannot execute extras or setup.

Ownership misses and interface-master differences are qualified observations.
Route/MTU comparisons use each saved flow separately and require observed ownership
in the requested context plus a selected installed route covering the destination.
Path comparison requires comparable global next hops; link-local next hops alone
cannot be compared to global trace responders. Known unique ownership can reconcile
two addresses of the same device. Multiple selected routes suppress path comparison.
A later route or lower egress MTU does not establish an earlier forwarding fault.
Reverse-path comparison is unavailable until the reverse parser is validated.

## Owner sweep

`--owner-sweep` lists addresses on inventory devices to resolve otherwise unmatched
hop addresses. `--site` and `--role` filter that sweep, not already matched devices.
Inventory ambiguity and explicit mappings retain their precedence. Duplicate owners
remain ambiguous; no device is chosen arbitrarily. All sweep command results,
including failures, are retained in `owner_sweep` in the report.

The address-only sweep closes its sessions before normal collection. Unique
matches then receive the requested groups through a second bounded pass with the
same credentials. This avoids retaining an inventory-wide set of idle connections
while ownership is reconciled. It is not an authentication retry or privilege
fallback. A unique match describes only successful snapshots in the selected
inventory scope; it is not proof that another unobserved owner does not exist.

## Sessions and bounds

Native SSH verifies target and jump host keys. Auth order is agent keys, then
explicit `--identity` files, or default `~/.ssh/id_ed25519`, `id_ecdsa`, `id_rsa`
when no identity is supplied. Missing/encrypted default files are skipped; explicit
missing/encrypted identities fail. Load encrypted keys into your agent yourself.
No passwords, interactive prompts, SSH-config interpretation or credential retries.
`--jump` supports one host, using the same user/authentication as the target.
`--insecure-host-key` is an explicit opt-out, never an automatic fallback.

Wrapper templates tokenize before substitution. Supported placeholders are
`{host}`, `{address}`, `{user}`, `{jump}`, `{platform}`, `{cmd}`. Exactly one `{cmd}`
occupies a whole argument. The literal executable resolves to an absolute path;
shell syntax, environment/command expansion and direct shell executables fail.
A supplied wrapper is trusted executable code, not a sandbox. Use company wrappers
only with their documented noninteractive behavior.

Defaults: four parallel devices, five-minute overall deadline, five-second SSH
connection deadline, 25-second command deadline, 1 MiB per stdout/stderr stream.
Maximums: 32 workers, 30-minute overall deadline, five-minute connection/command
deadlines, 256 devices per pass, 256 discovered interfaces per device, 32 extras.
Commands per device are serial; identical literal endpoints also serialize.
Different DNS aliases cannot be correlated by the offline planner.

Timeout/truncation closes native SSH with no reconnect. Wrapper cancellation
terminates its process group. Overall interruption stops scheduling, joins workers,
and saves partial evidence. A failed command is recorded without a retry loop.

## Output and evidence

Text, Markdown and JSON consume one computed report. The separate version-1
collection envelope embeds the unchanged saved-trace report. Each command has
host attribution through its device record, command/argv, UTC start/end, elapsed
seconds, exit status, raw stdout/stderr file digests and parsed facts. A fact's
`evidence` is a line/JSON pointer relative to that command's stdout. Finding
references include the raw filename. `trace.input` preserves original trace bytes.

Evidence uses a new directory (0700) and exclusive files (0600). Default directory
names are hidden `.tracefull-collection-*`, preventing Go from scanning raw captures
as packages. An existing explicit directory fails. Nothing silently overwrites it.
Evidence-write/finalization errors are returned. Keep raw device evidence private.

Exit 0 means at least one usable command and no overall failure; `status` still
shows partial coverage. Exit 3 means no usable commands or overall deadline expiry.
Usage/output/evidence errors return 2; interruption returns 130 (SIGTERM 143).
`complete` describes command coverage, not network health.

The terminal live view uses Bubble Tea with a spinner and per-device command state.
It defaults on only for text with terminal input/output, outside CI. JSON, Markdown,
pipes and `--from -` stay noninteractive. `--no-live` disables it; `--live` never
forces controls into machine output. During collection, `q`/Ctrl+C cancels and
preserves evidence. After collection, scroll with arrows, `/` toggles findings,
`e` shows the evidence path, and `q` returns to the shell and prints the final report.
Progress is best effort; slow UI consumption cannot block device work.

## Verification and remaining scope

Real authorized captures validate the Cumulus parsers; see
[fixture provenance](../internal/driver/cumulus/testdata/README.md). Tests replay
these scrubbed bytes through fake sessions/wrappers and use local synthetic SSH
servers for transport failures. Replay is not live verification of the new CLI.
No implementation test contacts a real device or sends diagnostic probes.

Full v0.2 still requires real captures for Linux/FRR, NX-OS, Junos, EOS, IOS-XE and
RouterOS, plus optics/reverse formats. Nondefault routing contexts are unsupported.
Collection configuration follows defaults < user config < project config <
`TRACEFULL_*` environment < explicit flags. Keys mirror this command’s flags;
unknown keys fail, and secrets are unsupported. Inventory SSH user/jump defaults
apply when the corresponding option is empty. `investigate` retains its separate
flag-only configuration contract. Publication and production verification of this
CLI are separate from local checks and capture validation.
