# tracefull — feature and fix workflow

Follow [AI rules](ai-interaction.md) for scope/authorization and
[coding standards](coding-standard.md) for engineering.

1. **Pending — document.** Record the problem, outcome, scope, acceptance criteria,
   affected packages, open decisions, and planned checks in
   [current-feature.md](current-feature.md), a shared brief rather than personal
   notes. During design, stop here; implement only when requested.

2. **In progress — implement.** Inspect and preserve working-tree changes. Create
   a feature/fix branch from the agreed base or reuse the appropriate branch.
   Build the smallest complete solution. Update the brief for agreed scope changes.

3. **Validating — test and iterate.** Add meaningful behavior/regression tests;
   reproduce bugs first where practical. Use manual checks where needed.
   Review the diff for correctness, scope, secrets, and documentation. Fix causes
   and rerun affected checks; record passes, failures, and limits. Mark `Blocked`
   with the specific missing input/access when progress depends on it.

4. **Ready for review — commit and integrate.** Once criteria/checks pass, prepare
   coherent commits, each reviewable within a 15-minute budget, with related tests
   and docs. Use concise imperative subjects. PRs explain problem/change,
   verification, and relevant issues; include screenshots for visible UI changes.
   Push, open PRs, and merge only within task authorization and repository policy;
   do not ask again for existing approval. Address review findings and check the
   final revision. Branch deletion requires explicit approval.

5. **Complete — verify and close.** Verify relevant CI and integrated behavior;
   record evidence, limitations, and commit/PR references in the brief. Finish
   the agreed scope before marking complete; pending review/merge stays
   `Ready for review`. Production deployment/testing require explicit
   authorization plus targets, validation steps, and a rollback plan.

Report implemented, tested locally, merged, and production-verified as separate
states supported by evidence. The review budget is a scope target, not a promise
about reviewer speed.
