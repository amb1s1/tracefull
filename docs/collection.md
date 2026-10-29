# Device collection status

v0.2 is in progress. There is no `collect` CLI or validated production driver yet.
The existing `investigate` command sends local-tool probes; it does not collect
device state over SSH.

## Implemented internal sessions

`internal/device` provides wrapper and SSH sessions. Both require a command
policy supplied in compiled code. There is no default allowlist, runtime plugin
or unknown-platform execution path. The policy must validate complete commands,
including parameters and setup exceptions. Current sessions reject control
characters, shell operators and pipelines; driver-owned pipelines are not yet
supported. A user-provided wrapper is trusted executable code, not a sandbox.

Wrapper templates are split into argv before substituting `{host}`, `{address}`,
`{user}`, `{jump}`, `{platform}` and `{cmd}`. Exactly one `{cmd}` occupies a whole
argument, including commands with spaces. Referenced target values must be
present and pass token validation. The executable is literal and resolved to an
absolute path. Shell syntax, environment/command expansion, unknown placeholders
and direct shell executables are rejected. `Plan` and `Run` use the same checks;
planning performs no execution or DNS lookup.

SSH sessions accept an explicit user, host:port, known-hosts file and agent socket
and/or identity paths. Agent keys precede file keys in one public-key auth method.
Encrypted identities requiring a password prompt fail; passwords and interactive
challenges are unavailable. Keys are limited to 1 MiB. Host-key checking is
mandatory unless the caller explicitly opts out. A single jump host uses its own
credentials and host-key policy. No credentials are silently changed or retried.

Connection timeout defaults to 5 seconds, command timeout to 25 seconds. A jump
and target handshake share the outer connection deadline. Session timeout values
must be positive and at most five minutes. Commands on a session run serially;
waiting callers can cancel. Each stdout/stderr stream is capped at 1 MiB. Results
retain timestamps, command, wrapper argv, raw output, status and exit code when
available. Timeout or truncation closes native SSH; that session cannot be reused.
Wrapper cancellation terminates its process group. There are no automatic
reconnects. The caller owns `Close`.

## Remaining v0.2 work and contract

Routing context defaults to the device's default table. Explicit VRFs must be
supported by the selected driver or fail; there is no silent fallback. Discovery
must resolve interfaces before dependent counters/optics/LLDP commands become
concrete. An offline preview must label those commands deferred.

Device identity, platform/version and scrubbed real command captures are required
before validating a driver. A ping response does not establish those facts.
Captures need the command, routing context, time and provenance, with addresses,
names and serials scrubbed before inclusion. Synthetic SSH server output tests
transport behavior only and must never be presented as device captures.

Platform policies/parsers, orchestration, evidence/report integration, owner
sweep, cross-device checks, CLI/configuration and the live view are unfinished.
Existing saved-report schemas remain unchanged. Single counter values will remain
cumulative; no rates or incident times may be inferred without supporting samples.

## Verification

Tests bind a synthetic SSH server to loopback, use generated ephemeral keys and
run local helper executables. They cover host-key rejection, authentication order,
jump forwarding, command rejection, deadlines, output caps, failures, cancellation
and preservation of captured output. No production SSH session was opened.
