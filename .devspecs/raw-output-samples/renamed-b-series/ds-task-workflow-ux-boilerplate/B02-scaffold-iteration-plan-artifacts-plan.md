# Task ds-task-workflow-ux-boilerplate B02 Plan

## Goal
scaffold iteration-plan artifacts

## Description
Create a bounded implementation slice for `Capture preferred ds task workflow UX for series, slices, iterations, lifecycle decisions, and learnings`. This plan is grounded by the B00 preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Resources
- `B00-index.md`
- `B02-scaffold-iteration-plan-artifacts-result.md`
- `task.json`
- `internal/commands/task.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/map.go`
- `internal/commands/capture.go`
- `internal/retrieval/retrieval.go`

## Starting Context
### Files to Inspect First
- `internal/commands/task.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/map.go`
- `internal/commands/capture.go`
- `internal/retrieval/retrieval.go`

### Tests to Inspect First
- None found. Search before editing.

## Expected Change Surface
- `internal/commands/task.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/map.go`
- `internal/commands/capture.go`
- `internal/retrieval/retrieval.go`

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.

## Risks
- Relevant tests may be missing from the initial pack.
- On-disk task anchors may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.

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
- [ ] Update `B02-scaffold-iteration-plan-artifacts-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
