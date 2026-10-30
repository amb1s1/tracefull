# Changelog

## Unreleased — collection foundations

- Add policy-checked wrapper and native SSH session primitives, with verified
  host keys, one jump host, bounded output, cancellation and synthetic local tests.
- Add `collect`, Cumulus 5.16 capture-backed parsers, two-phase interface discovery,
  owner sweep, private evidence, versioned output, qualified route/MTU comparisons
  and terminal collection progress.
- Add capture-backed Linux/FRR, Junos, EOS and IOS-XR drivers, per-lane DOM, explicit address
  context and MTU framing, and separate CPU utilization/load fields. Other planned
  drivers and reverse probing remain unavailable.
- Reject control characters in parsed IP zone identifiers; retain the fuzz failure
  as a saved regression case.

## Unreleased — investigation

- Add `investigate` orchestration for installed traceroute, MTR and Linux tracepath,
  with offline dry-run, bounded processes, saved evidence and partial-result reports.

## v0.1 milestone — merged, not released

- Import saved traceroute, tracert, tracepath, MTR, Paris and Dublin output.
- Match inventory addresses with explicit precedence and ambiguity.
- Add opt-in rDNS/ASN, qualified findings, text/JSON/Markdown reports and schema.
- Add local regression, schema, process, race, coverage, fuzz and vulnerability checks.

The saved-trace milestone merged through PR #1. Investigation merged through PR #2. Collection is under local
development/review. No release has been published.
