# Task scanless-workflow-ux A03 Plan

## Goal
Clarify find-before-task workflow and task context packing

## Description
Clarify when agents should use `ds find` before `ds task`, and when `ds task` should be the first command because it already refreshes and packs task context. The goal is not to forbid reconnaissance; it is to remove the false impression that users must run `find` before creating a task workspace.

## Resources
- `A00-index.md`
- `A03-clarify-find-before-task-workflow-and-task-conte-result.md`
- `task.json`
- `internal/commands/task.go`
- `internal/commands/map.go`
- `internal/scan/scan.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/find_pack_companions.go`
- `internal/commands/scan.go`
- `internal/commands/tldr.go`
- `internal/retrieval/retrieval.go`

## Starting Context
### Files to Inspect First
- `internal/commands/task.go`
- `internal/commands/map.go`
- `internal/scan/scan.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/find_pack_companions.go`
- `internal/commands/scan.go`
- `internal/commands/tldr.go`
- `internal/retrieval/retrieval.go`
- `internal/commands/eval.go`
- `internal/evalharness/eval.go`
- `internal/commands/find.go`
- `internal/commands/find_pack.go`

### Tests to Inspect First
- `internal/commands/map_test.go#L1440`
- `internal/scan/scan_test.go#L248`
- `internal/commands/task_test.go#L1903`
- `internal/commands/eval_test.go#L619`
- `internal/commands/find_pack_companions_test.go#L61`
- `internal/commands/find_pack_test.go#L206`
- `internal/commands/init_test.go#L13`
- `internal/scan/scan_timestamps_test.go#L18`
- `internal/commands/tldr_test.go#L34`
- `internal/retrieval/retrieval_test.go#L998`
- `internal/commands/retrieval_bridge_test.go#L63`

## Expected Change Surface
- `internal/commands/tldr.go`
- `internal/commands/find.go`
- `internal/commands/task.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`
- docs site quickstart/task pages, if mirrored before launch

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.
- Making `find` obsolete; it remains useful for brownfield reconnaissance and "what exists?" questions.
- Adding autonomous orchestration. This slice only clarifies workflow boundaries and hints.

## Risks
- Pack completeness is not high; verify the working set before editing.
- Overcorrecting agents away from legitimate exploration before task creation.
- Adding noisy hints after every `find` invocation.

## Product Decision
- `ds find` before `ds task` is acceptable when the agent is still discovering the repo, validating current intent artifacts, or checking whether a plan already exists.
- `ds task` is the preferred first command when the work item is already known, because it refreshes context and creates bounded slices with packed source/test/doc evidence.
- `ds map` remains the broader orientation command. `find` answers a question; `task` turns a known target into execution slices.
- Any CLI hint should be lightweight and contextual, for example after a packed `find`: "For implementation work, `ds task \"...\"` creates bounded slices with packed context."

## Success Criteria
- [ ] `ds tldr` gives agents a clear branch: orient with `map/find`, or start known work with `task`.
- [ ] README and task workflow docs say `ds task` already packs context for the generated slices.
- [ ] `find` help/output is reviewed for an optional next-step hint that does not create spam.
- [ ] Tests or docs snapshots are updated for any changed CLI text.

## Tasks
- [ ] Review current `ds tldr`, README, and task workflow docs for ambiguous ordering.
- [ ] Decide whether this is docs-only or needs a tiny `find` next-step hint.
- [ ] Update wording so "research first" and "task directly" are both legitimate paths.
- [ ] Run focused tests for any changed CLI output.
- [ ] Update `A03-clarify-find-before-task-workflow-and-task-conte-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
