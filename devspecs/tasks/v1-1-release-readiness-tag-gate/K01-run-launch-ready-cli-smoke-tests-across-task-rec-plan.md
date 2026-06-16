# Task v1-1-release-readiness-tag-gate K01 Plan

## Goal
Run launch-ready CLI smoke tests across task recent find map init and apply

## Description
Create a bounded implementation slice for `v1.1 release readiness and tag gate`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Run focused smoke tests for the v1.1 launch surface:
  - `ds task`
  - `ds recent`
  - `ds find`
  - `ds map` or `ds beta map`
  - `ds init`
  - `ds apply`
- Include both human-readable and JSON/prompt-output paths where relevant.
- Validate that `find`/`recent` are useful as diagnostic evidence and that task/apply are the main execution loop.

## Resources
- `K00-index.md`
- `K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md`
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
- [ ] A release smoke transcript or result artifact records exact commands and outputs judged launch-ready.
- [ ] Any beta-marker decision for architecture map or apply is recorded before docs/tag work.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
