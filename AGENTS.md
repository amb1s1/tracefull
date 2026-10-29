# tracefull — repository instructions

tracefull is a Go CLI under implementation for interpreting traces, identifying devices, and
collecting attributed read-only evidence. **Active scope: saved-trace enrichment
and local-tool investigation.**
v0.1 and local-tool investigation are merged. The user authorized continuation
into v0.2 device collection. Native probing remains design only. Collection
sessions are under implementation; device driver support requires real captures.

## Read only what the task needs

- Always read [AI rules](context/ai-interaction.md) and the
  [current feature](context/current-feature.md).
- Follow the [workflow](context/workflow.md) for feature and fix stages.
- Read [coding standards](context/coding-standard.md) for design or code changes.
- Consult relevant sections of the [specification](context/project-overview.md)
  for product contracts; it describes planned behavior, not working features.

## Scope and architecture

The first milestone is v0.1 saved-trace enrichment; collection is v0.2 and native
probing is v0.3. Stay within the requested milestone. Module path: `github.com/amb1s1/tracefull`; license: MIT.
Platform runtime support must be backed by recorded checks.

Use Command + Registry with compile-time composition: one package per subcommand,
explicit registration, no `init()` or runtime plugins. CLI adapters live in
`cmd/` and `internal/ui`; domain packages stay free of Cobra and terminal imports.

## Keep instructions maintainable

Each rule has one home: behavior in AI rules, engineering in coding standards,
delivery in workflow, product contracts in the specification. Keep selected
guidance self-contained; external repositories are research material, not required
reading or inherited instructions. Preserve licenses/notices when copying material.
Keep personal agent configuration and notes outside this repository.
