# Contributing

Start with [repository guidance](AGENTS.md) and the current feature brief.
v0.1 covers saved traces only; later collection/probing work needs its own scope.

1. Add a focused behavior/regression test and label synthetic data. Preserve
   provenance and license notices for sourced examples. Use RFC 5737/3849 addresses.
2. Keep parser and verdict code deterministic and free of I/O. Register dialects
   explicitly in `internal/importer`; put CLI adapters in their own command package.
3. Run `make tools`, `make check`, `make fuzz`, `make scan`, and `make cross`.
   A cross-build is not a runtime check. Do not weaken checks to obtain a pass.
4. Review changed goldens and schema compatibility. Keep commits small enough for
   a focused review. Report unavailable checks and runtime gaps explicitly.

New rules need threshold, missing-data, flow and conflicting-evidence tests plus
an `explain` entry. Driver contributions belong to v0.2 and require scrubbed real
device captures; synthetic data alone cannot validate a platform.
