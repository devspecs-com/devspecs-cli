# Task v1-1-agent-tooling-apply-loop J03 Plan

## Goal
Add ds apply prompt surface for next or explicit slice identifiers

## Description
Create a bounded implementation slice for `v1.1 agent tooling init and apply loop surfaces`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Add `ds apply next` and `ds apply <identifier>` as prompt-generation surfaces.
- Resolve task/track/slice identifiers to exactly one next actionable slice where possible.
- Emit an agent prompt that includes the slice goal, relevant files/artifacts, acceptance checks, and the rich completion contract.
- Do not launch external agents, tmux sessions, or background executors in v1.1.
- Keep this orthogonal to bespoke orchestration tools: the CLI provides the boundary prompt; other tools can run it.

## Resources
- `J00-index.md`
- `J03-add-ds-apply-prompt-surface-for-next-or-explicit-result.md`
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
- [ ] `ds apply next` never expands a whole track when a single next slice is available.
- [ ] `ds apply <identifier>` handles index, slice, and iteration identifiers with clear errors for ambiguous targets.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `J03-add-ds-apply-prompt-surface-for-next-or-explicit-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
