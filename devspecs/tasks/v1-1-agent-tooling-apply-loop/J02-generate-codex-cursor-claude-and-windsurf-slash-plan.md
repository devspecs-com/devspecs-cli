# Task v1-1-agent-tooling-apply-loop J02 Plan

## Goal
Generate Codex Cursor Claude and Windsurf slash command or skill files

## Description
Create a bounded implementation slice for `v1.1 agent tooling init and apply loop surfaces`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Generate launch-ready adapter files for Codex, Cursor, Claude, and Windsurf from shared templates.
- Prefer slash commands where the tool supports them and skills/rules where that is the native shape.
- Keep adapters thin: call `ds task`, `ds apply`, `ds find`, or `ds recent`; do not duplicate workflow logic in generated files.
- Include bounded-slice language so agents do not turn "proceed with A track" into implementing all slices.

## Resources
- `J00-index.md`
- `J02-generate-codex-cursor-claude-and-windsurf-slash-result.md`
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
- [ ] Each generated adapter has a predictable path, deterministic content, and a dry-run/test fixture.
- [ ] Adapter prompts point users toward `/ds:task "goal"` and `/ds:apply` as the main loop.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `J02-generate-codex-cursor-claude-and-windsurf-slash-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
