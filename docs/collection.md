# Device collection

`collect --from FILE|-` matches a saved trace to inventory and collects attributed,
read-only device snapshots. Drivers interpret recorded Cumulus Linux 5.16 / FRR
10.0, Junos 24.4R2-S5.3, Arista EOS 4.33.4M and Cisco IOS-XR 25.4.2 formats.
Cumulus also supplies the Linux/FRR validation environment. v0.2 is **in progress**,
not a completed multi-platform milestone.
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

## Drivers and interpretation

| Group | Cumulus implementation | Limits |
| --- | --- | --- |
| ingress | `ip -j address show` | Exact address ownership, not subnet ownership. |
| route | `vtysh -c 'show ip/ipv6 route ADDRESS json'` | Numeric destination, default context; retains prefix, protocol, selection, FIB state and scoped next-hop interface. IPv6 FRR output is capture-tested; IPv4 command construction is tested but its output variant has no live capture yet. |
| counters | `ip -j -s link show dev INTERFACE` | Owning and selected egress interfaces. Cumulative errors/drops, state and MTU; speed and CRC-specific counters remain unknown. The captured whole-link JSON validates the parser; per-interface selection is tested offline. |
| lldp | `/usr/sbin/lldpctl -f json` | One raw snapshot, parsed neighbors filtered to discovered interfaces. |
| version | `cat /etc/os-release`, `cat /proc/uptime` | Cumulus version and uptime seconds. |
| cpu | `cat /proc/loadavg` | Load averages, not utilization percentages. |
| optics, reverse | Explicitly unsupported | Require validated formats and, for live reverse probing, separate authorization. |

The other profiles and validated formats have distinct limits:

| Inventory platform | Captured formats | Interpretation limits |
| --- | --- | --- |
| `frr`, `linux` | Shared iproute2/procfs/FRR/lldpd formats validated on Cumulus Linux 5.16.0 / FRR 10.0.3; adds FRR `show version` | Both select the Linux/FRR profile. Requires `ip`, `vtysh` and lldpd for their groups. OS branding is not restricted to Cumulus, but other distributions/builds have no runtime validation. No kernel-route fallback if FRR is missing. |
| `junos` | Terse addresses, detailed connected IPv4 route, empty IPv6 lookup, extensive physical/logical counters, LLDP, version, uptime, master RE load and per-lane DOM JSON | Address routing-instance membership is unknown. Active RIB selection does not establish hardware installation. Physical MTU includes link framing. Logical owners also select their physical parent for observation. |
| `arista-eos` (`eos`) | IPv4/IPv6 interfaces, IPv6 static default route, counters, LLDP, version/uptime, CPU/load and Ethernet DOM JSON | Hardware-programmed state comes from explicit fields. CPU utilization has an unspecified sampling interval. SVI-to-physical-port mapping is unavailable, so an SVI alone cannot select uplink optics. Empty DOM remains unknown. |
| `cisco-ios-xr` (`iosxr`) | IPv4/IPv6 brief interfaces, connected IPv6 route, counters, LLDP, version/uptime, CPU utilization and per-lane optics text | Connected route and no-route layouts only; other route layouts fail with raw output retained. IPv6 context/prefix and hardware installation remain unknown. CPU percent is separate from load average. This is not an IOS-XE alias. |

The command policies accept only full reviewed command shapes. Fixed `| json` and
`| display json` suffixes are remote CLI formatting operations; additional pipes,
redirection and shell expansion are rejected. Junos, EOS and IOS-XR do not use
`--sudo`; Linux/FRR uses the same explicit `sudo -n` selection as Cumulus.
Their IPv4/IPv6 planners are tested offline, but captures establish only the
output variants listed above. Nondefault contexts fail before connecting.

Junos and IOS-XR optics preserve lane IDs, temperature in Celsius and RX/TX dBm;
missing readings remain null. EOS DOM uses its reported channel. These snapshots
do not diagnose a bad optic without thresholds and correlated evidence.
EOS Ethernet speed follows the `bandwidth` field used by Arista's
[interface-speed validation](https://anta.arista.com/v1.2.0/api/tests.interfaces/#anta.tests.interfaces.VerifyInterfacesSpeed).
IOS-XR speed comes from operational duplex/line rate, not configurable `BW`.
EOS CPU utilization is `100 - idle`; its 1/5/15-minute load averages are reported
separately, following the captured fields and
[Arista CPU interpretation](https://anta.arista.com/v1.3.0/api/tests/system/#anta.tests.system.VerifyCPUUtilization).
EOS/IOS-XR MTU framing is not established by these captures and is labelled unknown.
Only explicitly identified IP MTU participates in PMTU comparisons.

Default groups are `ingress,route,counters,lldp,version`. `--collect all` includes
unsupported groups, so coverage remains partial. Counter/optics/LLDP requests include
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

Real authorized captures validate the listed parsers; see fixture provenance for
[Cumulus](../internal/driver/cumulus/testdata/README.md),
[Linux/FRR shared validation](../internal/driver/linuxfrr/testdata/README.md),
[Junos](../internal/driver/junos/testdata/README.md),
[EOS](../internal/driver/eos/testdata/README.md) and
[IOS-XR](../internal/driver/iosxr/testdata/README.md). Tests replay
these scrubbed bytes through fake sessions/wrappers and use local synthetic SSH
servers for transport failures. Replay is not live verification of the new CLI.
No implementation test contacts a real device or sends diagnostic probes.

Full v0.2 still requires real captures for NX-OS, IOS-XE and RouterOS,
plus remaining optics/reverse formats. Nondefault routing contexts
are unsupported. Reverse probing requires separate authorization. Native SSH
execution of this CLI on real devices remains unverified; the recorded captures
used a separate bounded OpenSSH runner.
Collection configuration follows defaults < user config < project config <
`TRACEFULL_*` environment < explicit flags. Keys mirror this command’s flags;
unknown keys fail, and secrets are unsupported. Inventory SSH user/jump defaults
apply when the corresponding option is empty. `investigate` retains its separate
flag-only configuration contract. Publication and production verification of this
CLI are separate from local checks and capture validation.
