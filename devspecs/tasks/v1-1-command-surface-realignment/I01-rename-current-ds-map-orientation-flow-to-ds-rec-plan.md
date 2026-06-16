# Task v1-1-command-surface-realignment I01 Plan

## Goal
Rename current ds map orientation flow to ds recent with compatibility and docs

## Description
Create a bounded implementation slice for `v1.1 command surface realignment: task-first launch story, ds recent, and real architecture map`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Move the current recent-activity/orientation behavior out of `ds map` and into `ds recent`.
- Decide whether `ds map` remains as a temporary compatibility alias, a deprecated redirect, or is immediately reclaimed by I02. Record the decision explicitly.
- Update help, `ds tldr`, README/docs references, and tests so humans see `recent` for activity/trust evidence.
- Preserve the diagnostic job: recent commits, useful follow-up context commands, and evidence for what changed recently.

## Resources
- `I00-index.md`
- `I01-rename-current-ds-map-orientation-flow-to-ds-rec-result.md`
- `task.json`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`
- `devspecs/tasks/scanless-workflow-ux/A02-document-tldr-first-launch-workflow-and-two-laye-plan.md`

## Starting Context
### Files to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Tests to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Checkpoint Leads
Verify these prior checkpoint facts before widening search. They are not files the initial pack ranked as primary.
- `internal/commands/tldr.go` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `internal/commands/tldr.go`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `internal/commands/tldr.go`
- `internal/commands/tldr_test.go` [prior-test] - Verify this prior test lead before editing.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `internal/commands/tldr_test.go`
- `README.md` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `README.md`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `README.md`
- `TASK_WORKFLOW_EXAMPLE.md` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `TASK_WORKFLOW_EXAMPLE.md`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `TASK_WORKFLOW_EXAMPLE.md`
- `devspecs/tasks/scanless-workflow-ux/A02-document-tldr-first-launch-workflow-and-two-laye-plan.md` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated read `devspecs/tasks/scanless-workflow-ux/A02-document-tldr-first-launch-workflow-and-two-laye-plan.md`

## Expected Change Surface
- No pack-ranked primary file. Verify these checkpoint leads before choosing an edit target:
  - `internal/commands/tldr.go`
  - `README.md`
  - `TASK_WORKFLOW_EXAMPLE.md`

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.

## Risks
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/commands/map_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, map; `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, task.

## Success Criteria
- [ ] Primary implementation surface is verified before edits.
- [ ] Relevant tests are found or the test-surface miss is recorded.
- [ ] Changes stay inside the bounded slice.
- [ ] A checkpoint records actual files, tests, misses, noise, and decision.
- [ ] `ds recent` exposes the old orientation/recent workflow with launch-quality naming.
- [ ] Any remaining `ds map` recent-activity compatibility behavior is documented as intentional and temporary.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `I01-rename-current-ds-map-orientation-flow-to-ds-rec-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
