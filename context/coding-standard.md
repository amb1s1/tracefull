# tracefull — coding standards

These are self-contained project rules. The specification defines product
contracts and numeric limits. v0.1 implementation and local tooling are active; later collection/probing rules
remain requirements for their respective milestones.

## Architecture

- Each subcommand has its own package under `cmd/`, exposing
  `New() *cobra.Command`. Return fresh command/flag state. Commands translate
  CLI input into typed feature requests; feature logic belongs in `internal/<feature>/`.
- Root only registers and dispatches commands; main handles process lifecycle
  and exit status. Register commands explicitly in one place. No `init()`,
  Go plugins, HashiCorp go-plugin, or registration through import side effects.
- Parsers are separate dialect packages under `internal/parse/`, registered
  explicitly. Parsing and verdicts are deterministic and perform no I/O;
  shared domain types are allowed. Keep Cobra/terminal imports in `cmd/` and
  `internal/ui`, never in domain packages.
- Define small interfaces where consumed; return concrete types unless a real
  substitution boundary needs an interface. Wire dependencies explicitly.
  Avoid speculative frameworks, generic `utils`, and unnecessary public packages.
- Drivers own platform commands and parsers; device orchestration owns sessions,
  validation, limits, and evidence. Renderers consume one computed Report without
  mutating it or deriving findings. The versioned JSON schema is the output contract.

## Simple Go

- Use `gofmt`, conventional Go names, and comments explaining intent or protocol
  decisions. Choose the simplest correct code. No unused imports/variables or
  commented-out code unless requested.
- Aim for functions of at most 50 lines; split by responsibility, not arbitrary
  line counts. Explain a longer function when it is clearer.
- Prefer `net/netip` for addresses and `time.Duration` internally. Make serialized
  units and JSON/YAML tags explicit. Document slice/map ownership and copy when
  needed to prevent unintended mutation; avoid mutable global run state.
- Before substantial custom code, evaluate the standard library, existing
  dependencies, and maintained packages. Reuse only when total complexity falls;
  check API fit, license, maintenance, transitive dependencies, and Go compatibility.
  Pin versions when introduced; a planned dependency list is not an install list.
- Return contextual errors; use `%w`, `errors.Is`, and `errors.As` where appropriate.
  Handle/log a failure once at its owning boundary. No panic for bad input or
  network failure. Exit only in main, after application cleanup returns.

## Cancellation and resources

Pass `ctx` first through blocking calls; propagate cancellation instead of replacing
it with `context.Background()`. Keep configuration in typed parameters, not context
values, and call derived cancel functions.

Every goroutine and channel needs an owner, stop condition, and completion/close
path. Bound concurrency and close files, sockets, sessions, and child processes.
Cancellation stops scheduling, unblocks event delivery, closes resources, and waits
for workers. A slow/closed UI must not strand work. Test cancellation while blocked.

Bound cleanup and evidence finalization even after cancellation. Surface write/close
errors and preserve partial evidence. Set the interrupted-run exit contract before
implementation.

## Interpretation

Unknown is not zero. Preserve partial results and warnings; reject unrecognised
input. Findings need a rule, severity, supporting evidence, and limitations.
Centralize thresholds and rationale; retain sample counts, flows, and time windows.

Probe-response loss alone proves neither forwarding loss nor CoPP. RTT includes
return travel and reply generation. Route lookups and reverse traces are later
observations, not proof of an earlier packet's path. Apply specification §16's
qualifications to the draft verdict table.

Record inventory matching method and ambiguity. A configured address with a prefix
identifies that address, not ownership of the whole subnet; retain §8 precedence.

## Collection and evidence

- Use one planner for preview/execution. Validate every command, including setup
  and extras, against a platform policy. Unknown platform without a reviewed policy
  means no execution. No configuration or remediation commands.
- Prefixes alone are insufficient: validate substituted addresses/interfaces,
  reject shell control syntax, and allow only exact setup exceptions and explicitly
  modelled driver pipelines. Test accepted and rejected commands.
- Split wrapper templates into argv before substitution; preserve argument
  boundaries, including `{cmd}`. Never use a local shell/eval. Apply the same
  validation to device-side command parsing.
- Verify host keys by default; never silently weaken verification or retry with
  different credentials. No passwords/tokens in flags, config, logs, or fixtures;
  no automated password prompts. Serialize per-device commands, bound parallel
  devices, and enforce specified time, byte, and retry limits.
- Record host, command, argv, timestamps, elapsed time, exit status, output, and
  failures, including timeout/truncation/cancellation. Each parsed fact references
  raw evidence. Constrain paths to the evidence directory, avoid silent overwrite,
  and use restrictive permissions where supported.

## Output

Reports go to stdout; logs to stderr. JSON/Markdown contain no banners, progress,
or ANSI controls. Escape untrusted terminal controls and Markdown in views while
retaining raw evidence separately. Honour `NO_COLOR` and non-TTY output. Breaking
JSON changes require a schema version bump; test schema conformance.

## Verification and tooling

Keep `_test.go` beside code and scrubbed fixtures in `testdata/`. Label synthetic
data; real platform captures are required for driver validation. Test behavior
and regressions, not implementation trivia.

- Table-test verdict thresholds, low samples, missing hops, multiple flows,
  destination-only loss, and conflicting evidence.
- Golden-test parsers/reports with malformed/truncated input, IPv4/IPv6, multiple
  responders, annotations, and missing fields; fix timestamps and colour settings.
- Test planning, malicious substitutions, wrappers/setup, cancellation, output
  caps, partial failures, and evidence writes with fake sessions/local servers.
- Keep routine tests offline with mocked DNS/processes and no real credentials.
  Privileged probing requires an integration tag and authorized execution.
- Fuzz parsers with scrubbed/malformed seeds: check no panic, determinism, and valid
  successful results. Keep targets free of I/O; retain minimized failures. Normal
  tests run seeds; time-bound mutation runs and document exact target commands.

After module creation, baseline checks are:

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/tracefull
```

Document Go/tool versions, install/run commands, layout, environment variables,
sanitized config, and focused/golden-update commands in the README when added.
Do not assume custom test flags exist. Check Markdown links/format for docs changes.

The 90% verdict/parser coverage gates and lint checks are configured in the
Makefile; driver gates and release checks await their milestones. Checking-tool
versions and their update process are recorded in docs/dependencies.md. Vulnerability
database access is separate from offline tests. Report unavailable/failed scans
accurately; test runtime support before claiming it.
