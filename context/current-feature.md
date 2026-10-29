# v0.1 — saved-trace enrichment

## Status

In progress. Implementation authorized on 2026-10-29. Review and integration
remain separate from local completion.

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
- Go baseline and pinned dependency versions will follow inspected tool support.
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

## Pending contract decision

Asked once: DNS opt-in, optional default inventory versus explicit missing-file
failure, config precedence, and minimum 100 sent samples for compared loss rates.
Parser implementation can proceed independently.

## Verification evidence

Pending implementation. Host tooling: Go 1.27.3, darwin/arm64. No CI has run.

## History

- Project setup and boilerplate cleanup (existing documentation preserved).
- 2026-10-29: v0.1 implementation authorized; created codex/v0.1-enrich.
