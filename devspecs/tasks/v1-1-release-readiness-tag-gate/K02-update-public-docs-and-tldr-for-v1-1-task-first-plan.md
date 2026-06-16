# Task v1-1-release-readiness-tag-gate K02 Plan

## Goal
Update public docs and tldr for v1.1 task-first launch story

## Description
Create a bounded implementation slice for `v1.1 release readiness and tag gate`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Update README, `ds tldr`, docs site content, and release notes draft so the v1.1 story is consistent.
- Lead with `ds task`: task/slice/result, decision gates, and bounded agent work.
- Explain `ds find` and `ds recent` as diagnostics/evidence/trust tools.
- Explain `ds map` as architecture boundaries only if stable; otherwise document `ds beta map`.
- Include agent setup flow: run `ds init`, select tooling, then use `/ds:task "goal"`.

## Resources
- `K00-index.md`
- `K02-update-public-docs-and-tldr-for-v1-1-task-first-result.md`
- `task.json`

## Starting Context
### Files to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Tests to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

## Expected Change Surface
- Unknown. Identify the primary file before editing.

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.

## Risks
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Pack completeness is not high; verify the working set before editing.

## Success Criteria
- [ ] Primary implementation surface is verified before edits.
- [ ] Relevant tests are found or the test-surface miss is recorded.
- [ ] Changes stay inside the bounded slice.
- [ ] A checkpoint records actual files, tests, misses, noise, and decision.
- [ ] Docs do not teach stale `ds map` recent-activity semantics.
- [ ] Docs include one crisp next step after init and do not overpromise autonomous orchestration.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `K02-update-public-docs-and-tldr-for-v1-1-task-first-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
