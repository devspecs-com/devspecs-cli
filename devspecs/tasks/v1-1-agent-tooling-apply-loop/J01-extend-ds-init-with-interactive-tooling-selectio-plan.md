# Task v1-1-agent-tooling-apply-loop J01 Plan

## Goal
Extend ds init with interactive tooling selection and background indexing

## Description
Create a bounded implementation slice for `v1.1 agent tooling init and apply loop surfaces`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Extend `ds init` into a friendly setup flow that can initialize DevSpecs and offer agent tooling setup in one pass.
- Auto-detect likely Codex, Cursor, Claude, and Windsurf project/user config surfaces and preselect likely matches.
- Keep writes explicit: show what will be created or updated before writing agent command/skill files.
- Start or queue indexing in the background where feasible, but do not make background indexing hide errors that matter.
- End with exactly one recommended next step: `/ds:task "goal"`.

## Resources
- `J00-index.md`
- `J01-extend-ds-init-with-interactive-tooling-selectio-result.md`
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
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/commands/init_test.go` - on-disk path matched task terms but was not in the indexed candidate set: init; `internal/evalharness/agent_metrics_test.go` - on-disk path matched task terms but was not in the indexed candidate set: agent.

## Success Criteria
- [ ] Primary implementation surface is verified before edits.
- [ ] Relevant tests are found or the test-surface miss is recorded.
- [ ] Changes stay inside the bounded slice.
- [ ] A checkpoint records actual files, tests, misses, noise, and decision.
- [ ] `ds init` can run non-interactively for tests and scripted installs.
- [ ] Interactive output is sleek, selective, and does not bury the next action.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `J01-extend-ds-init-with-interactive-tooling-selectio-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
