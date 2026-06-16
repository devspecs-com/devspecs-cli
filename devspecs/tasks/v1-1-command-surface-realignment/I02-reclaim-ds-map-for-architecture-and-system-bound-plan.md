# Task v1-1-command-surface-realignment I02 Plan

## Goal
Reclaim ds map for architecture and system boundary output using boundary evidence

## Description
Create a bounded implementation slice for `v1.1 command surface realignment: task-first launch story, ds recent, and real architecture map`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Reclaim `ds map` for architecture/system boundary output instead of recent activity.
- Use the existing experimental boundary substrate as the starting point, then present cleaner subsystem blocks: purpose, boundary, evidence, adjacent systems, and suggested `ds find`/`ds task` follow-ups.
- Add a confidence gate. If output is unstable on recognizable OSS repos or this repo, expose the command as `ds beta map` instead of top-level `ds map`.
- Keep output legible for humans and pasteable for agents.

## Resources
- `I00-index.md`
- `I02-reclaim-ds-map-for-architecture-and-system-bound-result.md`
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
- [ ] Boundary output includes subsystem, purpose, boundary paths, evidence, adjacent systems, and follow-up commands.
- [ ] The release decision is explicit: stable top-level `ds map` or clearly marked `ds beta map`.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `I02-reclaim-ds-map-for-architecture-and-system-bound-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
