# v0.1 — saved-trace enrichment

## Status

Ready for review: v0.1, report readability and flow-attribution fixes are
implemented and verified locally. External review,
remote CI and integration remain pending; the overall feature is not Complete.

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
- No release, push, PR, deployment or production verification is authorized.

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
- CI: workflow configured with pinned action revisions; no remote CI ran.
  Linux/Windows native runtime and Windows console cancellation remain unverified.
- Merge/publication: no push, PR, merge, release or deployment occurred.
  Production/live-device verification: not performed; not authorized or needed
  for the saved-file milestone. DNS behavior is verified with fakes, not a live
  service availability test.
- No implementation blockers remain. Unsupported localized text, unrecoverable
  flow/sample metadata, and later-milestone features are documented in README.
  Next action: review the local branch; publishing/integration needs authorization.

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
