# Current feature — v0.2 device collection

## Active slice — explicit management VRF

Workflow at PR handoff: Ready for review on `codex/mgmt-vrf`.
[PR #4](https://github.com/amb1s1/tracefull/pull/4) tracks CI and integration.
The user requested continuation after a real
Cumulus collection completed with 14 successful commands and nine interfaces.
Its matched management address had master `mgmt`, while the default-context
lookup selected a fabric route. Raw captures remain private. PR #3 is merged as
`98e53aa`; all pre-merge and post-merge checks passed. The user-run collection
now provides live CLI evidence for the captured default-context Cumulus commands.

Scope and acceptance:

- Preserve default context unless explicitly selected. Add `--vrf mgmt` only to
  capture-validated Cumulus/Linux-FRR plans; other contexts/platforms continue to
  fail without fallback. No automatic context switching or new probes.
- Attribute addresses to VRFs only with interface-type/membership evidence.
  VRF-device addresses must not be labelled default solely because they have no
  master. Missing or conflicting context evidence remains unknown.
- Validate the management lookup backend against actual output before enabling
  it. Record kernel versus FRR interpretation limits; never hide a failed FRR
  lookup by silently substituting a different context or source.
- Keep route-driven interface discovery and comparisons within the requested context.
  Preserve command policy, raw evidence, existing schema and CLI/domain boundaries.
- Add capture-backed and failure tests, schema/CLI replay checks, race/vet/lint,
  coverage, parser fuzzing and documentation. Commit reviewed slices locally.

Implementation slices: correct existing unknown-context handling and ignore
private collection directories; validate new captures and implement typed context
handling; connect explicit management planning to collection and reporting; run
checks and review. The user granted standing read-only device authorization.
Bounded verified-host reads on October 30 (UTC) captured detailed addresses, FRR
management IPv6 and IPv4 lookups, and two kernel lookup formats. FRR returned an
explicit management context, so it remains the sole route backend. No fallback
or probe was added. Raw captures and identifying maps remain outside Git.

The independent uncertainty corrections are committed as `a0ae2f5` and `dab6db4`.
Management implementation adds detailed typed context attribution and exact
management command policies for Cumulus/Linux-FRR. A local IPv6 route selects
only eth0 in the capture replay. Unknown/mismatched FRR context fails parsing;
missing or conflicting interface metadata remains null. Existing schema and
default command shapes are unchanged. Unsupported contexts remain rejected.

Local validation passed: `make check` (tests, CLI replay, schema, formatting, vet,
race, coverage, staticcheck, golangci-lint, docs and build), `make scan cross`, and
two 30-second fuzz runs (676,059 Cumulus executions; 1,025,574 Linux/FRR).
Coverage: parsers 92.6%, verdicts 99.4%, drivers 92.8%. No reachable or
imported-package vulnerabilities; one unused required-module advisory remains.
Cross-builds cover Linux/macOS amd64/arm64 without claiming runtime verification.
Review covered context attribution, exact command policies, malformed output,
no-fallback behavior, evidence privacy, discovery and schema compatibility.

The actual native Cumulus CLI completed six read-only commands on October 30,
2026, 13:52:24–13:52:32 UTC. It reported the explicit management local IPv6 /128,
selected only eth0, and retained verified hashes for all 12 output streams.
No configuration changes or probes occurred. This validates that device/build
and command set, not other distributions, forwarding health or historical paths.
Raw evidence remains private. Driver and pipeline commits are `5a6d600` and
`8571b91`. PR #4 is authoritative for subsequent CI and merge status. No release
or deployment is included. Full v0.2 remains incomplete because of the platform
and format gaps recorded below.

## Status and acceptance

Local review completed against merged investigation `54e646f`, covering command
policy, session bounds, evidence attribution, partial results and parser
uncertainty. Three reproduced defects were corrected with regression tests;
results are recorded below. Collection delivery is tracked in
[PR #3](https://github.com/amb1s1/tracefull/pull/3); its checks and merge record
are authoritative for integration status. Full-milestone capture gaps remain open.

Workflow at PR handoff: Ready for review on `codex/v0.2-collect`.
v0.1 and investigation are merged; the user
explicitly authorized continuation into collection. No native probing is in scope.
Full v0.2 requires native/wrapper sessions, capture-validated platform drivers,
command policy, dry-run, evidence, owner sweep, cross-device observations and a
live view. Cumulus, Linux/FRR, Junos, EOS and IOS-XR slices are implemented; the full milestone
is not Complete. Remaining platforms and reverse formats need real captures.

## Implemented slices

- Explicit command and driver registries, policy-checked native SSH and wrapper
  sessions, agent-first/default/explicit keys, strict host keys, one jump, bounded
  serial commands and parallel devices. No password prompts, privilege fallback
  or automatic reconnects after command failure.
- Cumulus Linux 5.16 / FRR 10.0 capture-backed JSON/text parsers for addresses,
  route facts, counters, LLDP, OS version, uptime and load. Scrubbed real captures
  and provenance are committed with parser tests; synthetic failures are labelled.
- `collect` connects saved-trace matching to a shared offline/execution plan,
  two-phase interface discovery, owner sweep, private evidence and a separate
  version-1 collection envelope. Unknown platforms cannot execute commands.
- Qualified route/MTU comparisons keep flows separate and require comparable
  ownership/context/FIB evidence. Cumulative counters do not become rates.
  Link-local next hops retain interfaces; they do not imply global responder identity.
- Text/Markdown/JSON output; terminal progress and report viewport; config layers
  follow defaults < user config < project config < environment < flags.
  CLI adapters and UI remain separate from domain logic.

## Decisions and limits

The user selected default routing context unless an explicit VRF is supplied.
Unvalidated contexts fail without fallback. A matched interface master is not the
same as the requested lookup context. `--sudo` explicitly selects noninteractive
sudo for FRR/LLDP; a denial never changes credentials or privileges.

Owner sweep uses an address-only pass, closes those sessions, then collects unique
owners with the same credentials. This bounds open connections while deciding
ambiguity globally. The dry-run includes conditional follow-up commands. Existing
inventory ambiguity/mapping precedence remains intact; sweep uniqueness is scoped
to successful observations within the selected inventory/site/role.

Structured Cumulus output was chosen from the explicitly authorized real capture
commands, replacing the draft text-only proposal for this driver. This reduces
heuristics and preserves typed nulls and JSON-pointer evidence. Existing saved-trace
schema/version remains unchanged. The collection envelope has its own schema.

Selected-interface counters use the captured whole-link JSON format. The later
user-run native CLI collection also completed nine per-interface reads on the
Cumulus validation switch. IPv4 FRR command
planning is tested; management captures now cover IPv4 and IPv6 output. Other
nondefault VRFs, Cumulus optics and reverse traces remain unavailable. The three
new platform slices and remaining gaps are recorded below. Do not
present this slice as full v0.2 support.

## Capture authorization and evidence

The user authorized a bounded read-only Cumulus capture, then explicitly extended
it for noninteractive sudo FRR and LLDP reads. Strict host verification initially
failed. After the user trusted the IP against an existing hostname key, reads
succeeded. Unprivileged FRR/LLDP failures were preserved; approved sudo reads
supplied the missing formats. No configuration was changed and no reverse probes
were sent. Raw captures and identifying maps remain private outside Git.

Those initial captures are saved observations of the approved commands. The later
user-run native CLI validation is recorded separately in the active slice above.
The original trace and older root-owned evidence in
the user's checkout remain untouched. Development and checks use this worktree.

## Verification and delivery

### Local review corrections

- Equivalent endpoint spellings bypassed session serialization (for example,
  `192.0.2.1` versus `192.0.2.1:22`). The guard now normalizes host/port and IP
  spellings while preserving distinct ports and case-sensitive IPv6 zones.
  `TestEndpointAliasesShareSessionGate` failed before correction; queued aliases
  now wait or cancel, and can proceed after the first session closes.
- A named pipe selected as an identity or known-hosts file blocked SSH setup past
  its connection deadline. Those paths now require regular files, retaining
  symlinks to regular files. `TestSSHRejectsNamedPipeFiles` reproduced both hangs;
  it and the symlink/loopback-authentication tests now pass.
- Successful jump-host sessions returned an expected closed-transport error on
  cleanup, causing collection to report partial coverage. `TestSSHJump` now checks
  close success and reproduced the fault. Expected closed/EOF results from tunnel
  teardown are normalized; unexpected close errors remain visible and tested.

After corrections, `make check` passed on macOS: unit/CLI replay/schema tests,
formatting, vet, race, coverage, staticcheck, golangci-lint, Markdown validation and
build. Coverage gates passed: parsers 92.6%, verdicts 99.4%, drivers 92.5%.
The documented collection dry-run and schema commands passed with the built CLI.
`make scan` found no reachable or imported-package vulnerabilities; one advisory
remains in a required module outside imported packages. `make cross` compiled
Linux/macOS on amd64/arm64; those cross-builds are not runtime verification.
No additional device access or publication occurred during local review.
There are no unresolved findings from that review; the capture gaps below still
prevent full v0.2 completion. The reviewed branch was subsequently pushed and
PR #3 opened for Linux/macOS runtime and quality checks before merge.
The local review corrections are committed as `388ac35` and `4afbedd`.

### Implementation verification

Tests cover real scrubbed parser inputs, malformed/null fields, strict policies,
wrapper argv, host-key/authentication failures, jump forwarding, bounds, cancellation,
partial results, owner ambiguity, evidence-write errors, schema conformance and
actual CLI replay. Local terminal verification uses synthetic wrappers only.
Driver coverage now has the same 90% gate as parsers and verdicts; the new parser
has its own bounded fuzz target. Final check/fuzz/scan/cross results are recorded
below. Remote CI and integration results are attached to PR #3.

Local session commits: `e675258`, `abee317`, `7c08f40`, `f4002c4`, `c632d70`.
Cumulus/pipeline/UI slices passed local checks and are committed as `8933586`,
`bb75759`, `7606280`, `5362948`, `f363773`, `cf57394` and `cd09c08`.
Collection is submitted in PR #3. No release or deployment is included.
Full-milestone completion still needs additional platform captures. The user supplied and
authorized Junos, EOS and IOS-XR targets. Strictly verified reads recorded Junos
24.4R2-S5.3, EOS 4.33.4M and IOS-XR 25.4.2; raw captures remain private. IOS-XR
is an explicitly requested additional driver, distinct from the planned IOS-XE.
Those driver implementations follow the reviewed Cumulus slice. Linux/FRR was
subsequently validated from the same Cumulus captures at the user’s direction.
NX-OS, IOS-XE and RouterOS captures remain unavailable.

# Local tool investigation record

## Status

Complete: the investigation workflow merged through PR #2 as `54e646f`, with
Linux/macOS and quality checks passing at `36d0310`. The merged checkout passed
tests, build and offline IPv6 dry-run. v0.1 is Complete: PR #1 merged as `56c3e1c`
after review, Linux/macOS/quality CI passed at `ee8ca8b`, and the merged checkout
passed unit tests, build and an offline Dublin CLI smoke test. No release or
production verification occurred.

## Investigation scope and acceptance

1. `investigate TARGET` plans and sequentially runs available traceroute, MTR and
   Linux tracepath tools. Explicit compile-time registration; no shell, sudo,
   arbitrary command flags, SSH or native probe implementation.
2. Accept IP literals and hostnames; resolve hostnames once for the execution and
   pin one numeric address across tools. Dry-run performs no DNS or execution;
   unresolved hostname plans are explicitly deferred.
3. Bound the run to five minutes, each tool to two minutes, each stdout/stderr
   stream to 1 MiB, default max hops 30 and MTR cycles 100. Validate overrides.
   Cancel process groups and preserve partial evidence on interruption.
4. Create a new private evidence directory (0700) with exclusive files (0600),
   raw stdout/stderr, parser inputs and an investigation JSON report. Reject an
   existing explicit directory. Record argv, timing, exit status and failures.
5. Render one text/Markdown/JSON report with tool coverage and separate attributed
   per-tool reports. Do not combine samples across tools. Report unavailable,
   failed, timed-out, truncated and unparseable tools. Exit 3 if none succeeds;
   exit 0 when at least one succeeds, with partial coverage explicit. An overall
   deadline also uses exit 3 with partial evidence preserved. Usage/write errors
   use 2 and signal exits retain 130/143.

Implementation slices: planner and process runner; evidence and report pipeline;
CLI/schema/docs; offline tests, full checks and review. Use fake executables and
resolvers, never contact external targets during development. rDNS/ASN remain
opt-in. Investigation options use explicit flags; existing enrichment config
continues to apply only to `enrich`.

This feature moves external-tool orchestration ahead of the original collection
roadmap without authorizing device access or the remaining future milestones.
Paris/Dublin remain saved-input parsers until execution adapters are separately
implemented. There are no unresolved product decisions for this initial scope.

## Investigation verification — 2026-10-29

- `make check TOOLS_DIR=/tmp/tracefull-tools` passed unit and actual CLI tests,
  schema validation, existing goldens, formatting, vet, race, staticcheck,
  golangci-lint, Markdown checks and build. Parser coverage is 92.6%; verdict
  coverage is 99.4%, both above the existing 90% gates.
- `make fuzz` passed 2,610,424 executions in its 30-second fuzz window.
  `make scan TOOLS_DIR=/tmp/tracefull-tools` found no vulnerabilities.
  `make cross` compiled Linux/macOS on amd64/arm64; these are not runtime tests.
- Fake processes/executables verified bounded output, process-group cancellation,
  partial evidence, exclusive file creation, evidence digests and CLI exit codes.
  A regression test caught and corrected an embedded buffer method bypassing the
  output cap; the final runner exposes only the capped writer.
- The rebuilt CLI ran numeric and hostname dry-run examples and exported the
  investigation schema. Numeric dry-run found installed traceroute/MTR; hostname
  dry-run deferred execution without DNS. Tracepath is unsupported on macOS.
- No diagnostic packets, device access, publication or deployment occurred.
  Linux runtime, real-tool network behavior, new-feature CI and merge remain
  unverified. The next action is review of the local investigation commits.

### Review and user-run evidence

- The user supplied a macOS run on 2026-10-29: traceroute and MTR completed,
  while tracepath was correctly reported unsupported. All 17 MTR rows matched
  the supplied raw JSON for address, sample count, loss and displayed RTT values.
  This is user-recorded real-tool evidence, not an independently repeated run
  or a forwarding-cause diagnosis. Private captures are not committed.
- Review found that sudo-created private evidence in a non-hidden directory can
  make Go's recursive package discovery fail with permission denied. New default
  directories use `.tracefull-evidence-*`, which Go skips. Explicit paths and
  existing evidence are unchanged. A regression covers default report creation.
  Full checks run in a clean checkout; the original blocked invocation is not
  counted as a pass.
- Push, PR, CI and merge after successful review/checks are authorized, followed
  by work toward v0.2 collection. Real device captures and the routing-context
  contract are requested before implementing affected platform behavior.

## Completed v0.1 record

## Scope and acceptance

1. Import Linux/BSD/macOS traceroute, Windows tracert, tracepath, MTR text/JSON,
   Paris traceroute and Dublin JSON with deterministic parsing and explicit warnings.
2. Validate YAML/JSON inventories; attribute matches and ambiguity. Optional rDNS
   and ASN use bounded, cancellable lookups. No probing or device access.
3. Apply qualified verdicts with sample/flow safeguards. Preserve unknown facts.
4. Render one report as text, JSON and Markdown; expose explain, schema, version,
   inventory check, completion and help through explicit command registration.
5. Verify behavior, malformed inputs, schema, cancellation, offline integration,
   fuzzing, race, vet, formatting, coverage, vulnerability scan and builds.

## Implementation slices

1. Domain model, parser registry and dialects with regression tests.
2. Inventory validation/matching, DNS enrichment and verdict rules.
3. Report schema/rendering, configuration and CLI with terminal presentation.
4. End-to-end checks, documentation, review and focused local commits.

## Decisions and rationale

- Module: github.com/amb1s1/tracefull. License: MIT (explicit user decisions).
- Go baseline: 1.27.3. Dependency choices, compatibility corrections and pinned
  checking tools are recorded in [dependency choices](../docs/dependencies.md).
- Milliseconds for serialized RTT; bytes for MTU; unknown numeric/boolean facts
  are null. Stable ordering by TTL/address/flow; no invented measurement times.
- Imported raw data is attributed by SHA-256 and line/JSON references. DNS lookup
  time is distinct from the saved measurement window.
- Interrupted execution returns 130; termination returns 143. Invalid input and
  output failures return 2; configured findings return 1 after writing the report.
- Missing/truncated data cannot prove unreachable. Hop-limit takes precedence
  only when the imported tool explicitly identifies its configured limit.
- Destination-only loss takes precedence over real-loss. Different flow paths
  are evaluated separately. Intermediate response loss does not establish CoPP.
- Pushes, PR creation, review and merge after review/checks pass are authorized.
  No release, deployment or production verification is authorized.
- Runtime scope: Linux and macOS only (user decision, 2026-10-29). Windows
  execution is unsupported; importing saved `tracert` text remains in scope.

## v0.1 contract choices

- DNS is opt-in (`--rdns`, `--asn`); `--no-rdns` can disable an inherited setting.
  This keeps saved-file analysis offline unless the user requests lookups.
- Missing default inventory is allowed; explicit missing inventory is an error.
- Precedence: defaults < user config < project config < environment < flags.
- Loss classification requires at least 100 sent samples at every compared hop,
  known destination response and no merged responder braid. This is a sample floor,
  not a statistical confidence guarantee. Qualified names follow specification §16.

The user explicitly approved these decisions on 2026-10-29.

## Verification evidence

Local verification on 2026-10-29, macOS 27.0.1 arm64, Go 1.27.3:

| Check | Result |
| --- | --- |
| `make check TOOLS_DIR=/tmp/tracefull-tools` | Passed: unit/regression/golden/schema/process tests, gofmt, vet, race, coverage, staticcheck, golangci-lint, Markdown checks and build. |
| Parser / verdict coverage | 92.3% / 99.3%; both independent 90% gates passed. |
| `make fuzz` | Passed: final 30-second mutation run completed 1,647,085 executions. No failure corpus was produced. |
| `make scan TOOLS_DIR=/tmp/tracefull-tools` | Passed: govulncheck v1.9.0 reported no vulnerabilities after patching dependencies. |
| `make cross` | Passed: CGO-disabled linux/darwin/windows × amd64/arm64 compilation. No runtime claim for those cross-builds. |
| Actual CLI | Documented text/JSON/Markdown examples; explain, schema, version, Bash completion and manpage output passed. JSON was decoded; pipes contained no ANSI and stderr was clean. |
| Process failure/cancellation | Tests verified exit 0/1/2, report preservation for fail-on, and exit 130 while blocked on stdin and a full stdout pipe. |
| Terminal | Theme/Glamour unit tests passed; real executable in a local pseudo-terminal rendered and exited 0. A nonresponding terminal took about four seconds for background-color query fallback. |
| Source/diff review | Reviewed parser contracts, uncertainty, config precedence, output escaping, cancellation ownership, fixture provenance and staged whitespace checks. |

A first pseudo-terminal harness falsely reported failure by checking process state
before waiting for exit after terminal closure. The corrected bounded check
waited for completion and passed. The executable's stdin/stdout cancellation tests
remain part of the automated suite.

## Delivery and remaining limits

- Implemented: every v0.1 slice above, including all seven dialect families,
  explicit parser/command registries, MIT license and approved module path.
- Local commits: focused domain, parser, matching, verdict, schema, configuration,
  pipeline, UI and CLI changes on `codex/v0.1-enrich`. See `git log --oneline`.
  Key implementation commits: `f989f06`, `7831ec5`, `dc433dc`, `dfd2650`,
  `fbb43d5`, `097bc13`, `d9f0885`, `822f559`, `b96307c`.
- CI: at `04668cd`, Linux and macOS runtime jobs passed race tests, vet and build.
  Windows failed parser/report golden comparisons and is outside the newly
  selected runtime scope. The quality job passed; the run failed solely on Windows.
  Latest full-run status is recorded below.
- Delivery: `codex/v0.1-enrich` was pushed to origin with user authorization.
  No PR, merge, release or deployment occurred.
  Production/live-device verification: not performed; not authorized or needed
  for the saved-file milestone. DNS behavior is verified with fakes, not a live
  service availability test.
- No implementation blockers remain. Unsupported localized text, unrecoverable
  flow/sample metadata, and later-milestone features are documented in README.
  Next action: verify the revised CI matrix; review/integration remain pending.

## History

- Project setup and boilerplate cleanup (existing documentation preserved).
- 2026-10-29: v0.1 implementation authorized; created codex/v0.1-enrich.

- 2026-10-29: completed the v0.1 pipeline, corrected dependency vulnerabilities,
  parser edge cases, alias precedence and Unix stream cancellation; local checks
  passed. Marked Ready for review, with CI and platform-runtime gaps explicit.

## Follow-up regression — multiline traceroute

- Accept indented responder continuations as part of the preceding TTL, including
  IPv6 and hostname/IP rows. Recompute statistics over every recorded probe and
  retain each contributing line reference. Keep unrecognized extensions visible.
- Limit text/Markdown numbers to three fractional digits; keep JSON precision.
- Verify the scrubbed real capture has 15 hops, three probes at every hop, and
  multiple responders at TTLs 9, 11, 13, 14 and 15. Add synthetic boundary tests.
- Rebuild the local binary and reprocess the original file offline. Preserve the
  user's untracked `trace.txt`; commit only code, scrubbed fixture and documentation.

Verification on 2026-10-29:

- Reproduced skipped responders and IPv6 continuation misclassification with
  failing regression tests before correcting the parser.
- `make check TOOLS_DIR=/tmp/tracefull-tools` passed after the fix, including
  schema/goldens, CLI integration, race, formatting, vet and both linters.
  Parser coverage is now 92.5%; verdict coverage remains 99.3%.
- `make fuzz` passed 2,131,713 executions with the scrubbed capture added as a
  seed. No failure corpus was produced.
- `make scan TOOLS_DIR=/tmp/tracefull-tools` reported no vulnerabilities;
  `make cross` compiled all six candidate platform/architecture combinations.
  Cross-compilation does not verify runtime behavior.
- Rebuilt `bin/tracefull` and reprocessed the original capture with DNS disabled:
  15 hops, 45 probes, no skipped lines, and multiple-responder observations at
  TTLs 9, 11, 13, 14 and 15. Three samples do not support a loss classification;
  multiple responders alone do not prove ECMP.
- Text/Markdown rounding and unchanged JSON precision have regression coverage.
  The original untracked capture remains untouched and outside the commits.

## Follow-up — multiple-responder finding clarity

- Use an observation-based display label and explain that multiple responders
  do not establish ECMP or a network fault. Preserve JSON kind `ecmp`, rule
  `v0.1/ecmp`, severity `warn` and `--fail-on ecmp` compatibility.
- Attribute a finding at one TTL only to that hop's evidence; retain supporting
  evidence for comparisons across flow paths.
- Update text/Markdown output, `explain` and README. Verify rendering, JSON and
  exit-code compatibility, run local checks and rebuild the executable.
- No unresolved product decisions; this changes explanation and attribution
  within v0.1, without changing detection or adding investigation tools.

Verification on 2026-10-29:

- `make check TOOLS_DIR=/tmp/tracefull-tools` passed: unit/regression, schema,
  golden and CLI tests, race, formatting, vet, linters, docs and build. Parser
  coverage remains 92.5%; verdict coverage remains 99.3%. No goldens changed.
- `make scan TOOLS_DIR=/tmp/tracefull-tools` reported no vulnerabilities.
- The rebuilt executable reprocessed the original capture offline in text,
  Markdown and JSON; all five findings retained their identifiers and severity.
  Hop 15 references only lines 22–24. `explain` uses the clearer label, and
  `--fail-on ecmp` still returns exit 1 after writing the report.
- The original capture's digest is unchanged. Remote CI, integration and platform
  runtime gaps remain as recorded above; no new probes or device access occurred.

## Follow-up — local report and evidence review

- Compact repeated unmatched identities and duplicate address/name entries in
  text/Markdown. Preserve matched and ambiguous address attribution and JSON.
- Show limitations shared by every finding once; keep finding-specific caveats
  attached to their findings. Preserve all per-finding limitations in JSON.
- Correct cross-flow observations to identify the first TTL with differing known
  responders and cite the compared flow evidence. Missing replies or different
  capture lengths alone must not establish path variation.
- Verify regression cases, schema/goldens, complete local checks and actual CLI
  rendering; rebuild and make focused commits. No new milestone is authorized.

Verification on 2026-10-29:

- Failing tests reproduced cross-flow findings at the wrong TTL and false path
  differences caused solely by missing replies, shorter captures or empty flows.
  These cases now pass; evidence identifies the compared flow observations.
- `make check TOOLS_DIR=/tmp/tracefull-tools` passed, including race, schema,
  goldens, integration, formatting, vet and linters. Parser/verdict coverage is
  92.5%/99.4%. No golden snapshots were changed.
- `make scan TOOLS_DIR=/tmp/tracefull-tools` found no vulnerabilities. `make cross`
  compiled all six targets; this is not runtime verification on those platforms.
- The rebuilt CLI processed the original capture in text, Markdown and JSON:
  15 compact unmatched rows, five findings, one shared limitation in human output,
  and all per-finding limitations retained in JSON. Original capture unchanged.
- Synthetic Dublin variants exercised the actual CLI: a changed responder was
  attributed to TTL 2 and both flow references; a missing reply and a shorter
  capture produced no cross-flow finding. These are tests, not live observations.
- Review/CI/integration remain pending. No publication, deployment or live-device
  verification occurred. Next action: review the local branch.

## Follow-up — Linux and macOS runtime scope

- The user explicitly excluded Windows runtime support on 2026-10-29 after the
  first remote run failed on `windows-latest`. This is a product scope decision,
  not an assertion that Windows tests passed.
- Remove Windows from the runtime matrix and cross-build targets; retain all
  Linux/macOS tests and gates. Keep `tracert` import and its regression fixtures.
- Align README, dependency notes and the specification with Linux/macOS scope.
  Existing Windows fallback files and transitive dependencies need no deletion.
- Verify docs, four cross-build targets and the local check suite; commit, push
  and inspect the new CI run. No unresolved decisions.

Verification on 2026-10-29:

- `make check TOOLS_DIR=/tmp/tracefull-tools` passed unchanged Linux/macOS gates
  locally, including parser/report goldens and saved `tracert` fixtures.
- `make cross` passed all four Linux/macOS × amd64/arm64 builds.
- Prior remote run `40958720082` completed: Linux runtime, macOS runtime and
  quality passed; Windows runtime failed. Its failure is retained as historical
  evidence. A fresh run is required for the revised platform matrix.

## PR review

- Remote run `40959330516` passed all three Linux/macOS/quality jobs at `059a434`.
- Created `main` at the documentation baseline `0b10854` and opened PR #1 against
  it. Review covers interpretation, attribution, config precedence, offline DNS
  defaults, input bounds, output escaping, cancellation and explicit registration.
- Found MTR continuation addresses missing their evidence line. The correction
  adds the reference and tests its propagation into findings. The single affected
  synthetic golden is intentionally updated to include `line:3`; no measurements
  or other snapshots change.
- Await final local and PR checks before merging. The user also authorized the
  next feature: orchestration of local tools through an investigation command.

## Cumulus slice verification — 2026-10-29

`make check TOOLS_DIR=/tmp/tracefull-tools` passed after fixing one lint message
and an integration-fixture XDG directory collision with the test binary. Tests
include unit/CLI/schema/golden checks, race, vet, formatting, both linters, docs
and build. Coverage: parsers 92.6%, verdicts 99.4%, drivers 98.5%; collection
78.8% and session 88.2% are supplementary measurements, not new gates.

`make fuzz` passed 2,098,741 importer and 2,562,625 Cumulus executions. A follow-up
Cumulus fuzz run adds actual scrubbed captures as seeds. `make scan` found no
reachable/imported-package vulnerabilities; the unused OpenPGP module advisory
remains. `make cross` compiled Linux/macOS amd64/arm64, without runtime claims.

The documented offline dry-run and embedded schema commands ran successfully.
A local pseudo-terminal replay verified progress, final viewport, q-to-exit,
exit 0 and a complete evidence report. It used a synthetic wrapper and sent no
network traffic. The new CLI has not been run against production devices.

## Junos, EOS and IOS-XR slice — 2026-10-29

Scope: the user supplied and authorized three targets. Bounded, strict-host-key
OpenSSH reads captured Junos 24.4R2-S5.3, EOS 4.33.4M and IOS-XR 25.4.2 without
configuration changes, sudo or probes. Raw bytes and anonymization maps remain
private outside Git. Scrubbed fixtures preserve source hashes/times and mark
excerpts; omitted rows cannot establish device-wide absence.

Acceptance: explicit platform registration, exact read-only command policies,
default context without fallback, capture-backed parsers, private attributed
evidence, pipeline/CLI replay, failure and unknown-value tests, schema conformance,
90% driver coverage, bounded fuzzing and full local checks. Implementation slices:
shared facts/policy; one driver per platform; pipeline reconciliation; docs/checks.

Decisions: IOS-XR is distinct from IOS-XE. Fixed remote JSON display suffixes are
allowed only under complete platform policies. Address families are reconciled
together before ownership misses are reported. Junos terse and XR IPv6 brief
context remains null; Junos physical MTU is link-layer, and EOS/XR framing remains
unknown. RIB selection cannot imply hardware installation. XR dynamic route
layouts fail explicitly until captured. Optics remain per lane, and CPU load and
utilization use separate fields. No guessed aggregation or SVI uplink mapping.

Local verification passed (details below). Initial capture replay exposed EOS address-field shape
differences, now corrected. Importer fuzzing found a control byte accepted in an
IPv6 zone identifier; normalized address validation now rejects it, preserving
safe zone names and the minimized regression corpus. No checks were weakened.

Remaining blockers: NX-OS, IOS-XE and RouterOS captures; unvalidated
command variants, remaining DOM formats and reverse traces. The new CLI has
not been production-tested. Collection has not been pushed or run in remote CI,
merged, published or deployed. Full v0.2 remains incomplete.

Verification results for the multi-platform slice:

- `make check TOOLS_DIR=/tmp/tracefull-tools` passed unit/CLI/schema/goldens,
  formatting, vet, race, staticcheck, golangci-lint, docs and build. Coverage:
  parsers 92.6%, verdicts 99.4%, drivers 92.0% before the final EOS CPU addition.
- Full fuzz run passed after retaining/fixing the importer failure: 2,578,170
  importer, 1,129,889 Cumulus, 132,417 Junos, 897,597 EOS and 356,040 IOS-XR cases.
  The reviewed EOS SVI speed correction passed another 611,784 mutations.
- Vulnerability scan found no reachable or imported-package vulnerabilities;
  one advisory remains in an unused required module. Four Linux/macOS cross-builds
  passed; these are compilation checks, not runtime verification.
- Actual CLI replay covered each new platform in text, Markdown and JSON.
  Pipeline replay checked two-phase discovery, parent interfaces, per-lane DOM,
  address-family reconciliation, private evidence and strict schema conformance.
  The documented dry-run and collection schema commands also passed offline.
- Fixture privacy review checked all source hashes against retained private bytes.
  IOS-XR fixture line endings/trailing padding are normalized for repository diffs;
  tests verify equivalent LF/CRLF parsing, with original bytes retained privately.
- EOS CPU was independently finishable using the already authorized target. One
  bounded `show processes top once | json` read supplied a thirtieth capture;
  only CPU/time fields are retained in the scrubbed excerpt. Reported utilization
  is 100 minus idle with unknown sampling interval; load averages remain separate.
  Final checks after this addition are recorded below.

Review corrections include full platform policy after fixed JSON suffix handling,
unknown FIB/context qualifications, SVI speed remaining null, distinct CPU units,
Junos malformed nested JSON rejection and source-attributed route goldens.
Local commits: `a4254a1` (importer regression), `762fa58` (shared facts/policy),
`088f96a` (Junos), `1bbcf9b` (IOS-XR), `38774db` (EOS), `fffd997` (integration).
The user confirmed that no captures or targets are available for the remaining
four platform families. They remain blocked; do not fabricate fixtures or claim
platform support. No new production configuration, probing or CLI execution occurred.

## Linux/FRR validation decision

The user explicitly selected the same NVIDIA Cumulus switch for Linux/FRR because
it runs FRR. This resolves the Linux/FRR capture gap. Inventory `linux` and `frr`
select one explicit Linux/FRR profile, sharing the tested iproute2/procfs/FRR/lldpd
commands and parsers with Cumulus. The version group adds the already captured
FRRouting 10.0.3 identity. OS-release branding is independent from the Cumulus
profile's strict identity check. No new device connection was needed.

Validation remains scoped to Cumulus Linux 5.16.0, Linux 6.1 and FRR 10.0.3;
this does not claim runtime verification on every Linux distribution or FRR build.
Missing FRR/lldpd and permissions remain explicit command failures; no guessed
kernel-route fallback or privilege escalation. Both profile names are tested
through the full evidence pipeline and actual CLI replay. The remaining capture
blockers are NX-OS, IOS-XE and RouterOS, plus the previously recorded missing
formats and reverse probing. The user has no targets/captures for those platforms.

## Pre-integration verification and handoff

Local handoff state was Ready for review on `codex/v0.2-collect`. The final Linux/FRR revision passed
`make check TOOLS_DIR=/tmp/tracefull-tools`: unit, regression, CLI, schema/golden,
formatting, vet, race, staticcheck, golangci-lint, docs and build. Coverage is
92.6% parsers, 99.4% verdicts and 92.5% drivers. All six parser fuzz targets passed
bounded runs; the final EOS CPU run passed 875,681 cases and Linux/FRR passed
437,321. The minimized importer failure is committed and now passes.

The final vulnerability scan found no reachable or imported-package issues; one
unused required-module advisory remains. Final Linux/macOS amd64/arm64 builds
passed without asserting runtime execution. CLI replay covers every profile;
an additional EOS CPU text/JSON replay verified 3.1% reported utilization with an
unknown interval and separate 0.73/0.65/0.6 load averages from the saved capture.
These are offline replay results, not current device health.

EOS CPU is committed as `8ca0f0b`; Linux/FRR as `749c2a2`. All 30 new platform
fixture provenance entries were checked against retained private source hashes;
shared Cumulus fixtures retain their existing provenance. No raw private captures,
credentials or identifying maps were staged. IOS-XR whitespace normalization is
explicitly documented and tested, rather than disabling whitespace checks.

At this pre-integration handoff, collection CI, push and merge had not occurred.
The later local review corrections and PR #3 delivery supersede that handoff
state. The new CLI has not been exercised on production devices; read-only
capture observations remain separate from local implementation tests.
Full v0.2 remains blocked on the unavailable platforms and recorded format gaps;
do not advance to native probing or mark the milestone Complete. Platform work
resumes when real captures or authorized targets are supplied. No release or
deployment is part of this delivery.
