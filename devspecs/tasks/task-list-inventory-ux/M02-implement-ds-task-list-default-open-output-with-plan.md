# Task task-list-inventory-ux M02 Plan

## Goal
Implement ds task list default open output with --all --closed and --since filters

## Description
Create a bounded implementation slice for `Task list inventory UX: list open plans by default, support all closed since filters, and remove or retire old artifact list from public launch surface`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Resources
- `M00-index.md`
- `M02-implement-ds-task-list-default-open-output-with-result.md`
- `task.json`
- `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`
- `internal/commands/init_test.go`
- `internal/commands/init.go`
- `internal/initflow/initflow.go`
- `internal/commands/scan.go`

## Starting Context
### Files to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Tests to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Checkpoint Leads
Verify these prior checkpoint facts before widening search. They are not files the initial pack ranked as primary.
- `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`
- `internal/commands/init_test.go` [prior-test] - Verify this prior test lead before editing.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/init_test.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated edited `internal/commands/init_test.go`
- `internal/commands/init.go` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/init.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated edited `internal/commands/init.go`
- `internal/initflow/initflow.go` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/initflow/initflow.go`
- `internal/commands/scan.go` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/scan.go`; task task-freshness-sync-trust checkpoint cp_20260615T185505Z_b04_validated read `internal/commands/scan.go`

## Expected Change Surface
- No pack-ranked primary file. Verify these checkpoint leads before choosing an edit target:
  - `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`
  - `internal/commands/init.go`
  - `internal/initflow/initflow.go`

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.

## Risks
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/adapters/openspec/openspec_test.go` - on-disk path matched task terms but was not in the indexed candidate set: open; `internal/commands/golden_test.go` - on-disk path matched task terms but was not in the indexed candidate set: old.

## Success Criteria
- [ ] Primary implementation surface is verified before edits.
- [ ] Relevant tests are found or the test-surface miss is recorded.
- [ ] Changes stay inside the bounded slice.
- [ ] A checkpoint records actual files, tests, misses, noise, and decision.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `M02-implement-ds-task-list-default-open-output-with-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
- Block: external input or a missing prerequisite prevents useful progress.
