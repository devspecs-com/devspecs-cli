# Task v1-1-command-surface-realignment

## Task
v1.1 command surface realignment: task-first launch story, ds recent, and real architecture map

## Status
packed

## Series
I

## Profile
code-change

## Created At
2026-06-16T08:09:52Z

## Original Query
v1.1 command surface realignment: task-first launch story, ds recent, and real architecture map

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/v1-1-command-surface-realignment`

## Resources
- `task.json`
- `I01-rename-current-ds-map-orientation-flow-to-ds-rec-plan.md`
- `I01-rename-current-ds-map-orientation-flow-to-ds-rec-result.md`
- `I02-reclaim-ds-map-for-architecture-and-system-bound-plan.md`
- `I02-reclaim-ds-map-for-architecture-and-system-bound-result.md`
- `I03-make-task-workflows-the-main-launch-story-across-plan.md`
- `I03-make-task-workflows-the-main-launch-story-across-result.md`
- `I04-harden-rich-task-result-completion-contract-for-plan.md`
- `I04-harden-rich-task-result-completion-contract-for-result.md`

## Task Slices
- I01: Rename current ds map orientation flow to ds recent with compatibility and docs. Plan: `I01-rename-current-ds-map-orientation-flow-to-ds-rec-plan.md`. Result: `I01-rename-current-ds-map-orientation-flow-to-ds-rec-result.md`.
- I02: Reclaim ds map for architecture and system boundary output using boundary evidence. Plan: `I02-reclaim-ds-map-for-architecture-and-system-bound-plan.md`. Result: `I02-reclaim-ds-map-for-architecture-and-system-bound-result.md`.
- I03: Make task workflows the main launch story across tldr help README and docs. Plan: `I03-make-task-workflows-the-main-launch-story-across-plan.md`. Result: `I03-make-task-workflows-the-main-launch-story-across-result.md`.
- I04: Harden rich task result completion contract for promote improve iteration loops. Plan: `I04-harden-rich-task-result-completion-contract-for-plan.md`. Result: `I04-harden-rich-task-result-completion-contract-for-result.md`.

## Product Decisions
- `ds task` is the launch story. It creates bounded slices, keeps decision gates visible, and should be the first workflow in `ds tldr`, README, docs, and agent prompts.
- The current `ds map` orientation/recent-activity view should become `ds recent`. It is useful, but its job is diagnostic: "what changed recently and what evidence should I trust?"
- `ds find` and `ds recent` should be positioned as evidence and trust layers around task work, not as the main workflow users are expected to start with when they already have a goal.
- Reclaimed `ds map` should mean architecture/system boundary mapping from the full index: subsystem, purpose, path boundary, evidence, adjacent systems, and suggested follow-up commands.
- If boundary quality is not stable enough for mined OSS repos, ship the architecture view under `ds beta map` until confidence is good enough for the top-level command.
- Completion results should preserve the `M00`/`M01`/`M01-1` mental model: index, planned slice, improvement iteration after evidence, and explicit promotion/rework/rollback decisions.

## Current Capability Read
- The old map substrate already has experimental boundary support via `ds map --experimental-boundaries`; it uses path clusters, imports, tests, docs, and recent commits.
- The output is not yet the desired product shape. It needs cleaner subsystem language, purpose inference, adjacency display, and a stable confidence/beta gate.
- The task result/checkpoint machinery already captures many rich result fields, but the generated result template does not yet force the full completion contract.

## Relevant Map Areas
No strong map area was inferred from the initial pack.

## Likely Primary Files
None found in the initial preflight.

## Likely Tests
None found in the initial preflight.

## Likely Docs / Plans / Config
None found in the initial preflight.

## Supporting Context
None found in the initial preflight.

## Related Git Receipts
None found from packed paths.

## Noise Risks
None found in the initial preflight.

## Freshness Warnings
These on-disk paths match the task wording but were not present in the indexed candidate set. Treat them as stale-index risk, not proof that the initial pack is wrong.

- `internal/commands/map_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, map
- `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, task
- `internal/commands/acceptance_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command
- `internal/commands/db_error_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command
- `internal/commands/eval_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command
- `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command
- `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command
- `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/map_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, map; `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command, task; `internal/commands/acceptance_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command; `internal/commands/db_error_test.go` - on-disk path matched task terms but was not in the indexed candidate set: command

## Checkpoint Leads
The current pack is weak, so these compact checkpoint facts are verification leads only. They are not pack-ranked edit targets.

- `internal/commands/tldr.go` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `internal/commands/tldr.go`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `internal/commands/tldr.go`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated learned: Known work should start with ds task or ds task quick; map/find are discovery steps before a target is concrete, not mandatory pre-task setup.
- `internal/commands/tldr_test.go` [prior-test, checkpoint_fact]
  Agent check: Verify this prior test lead before editing.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `internal/commands/tldr_test.go`
- `README.md` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `README.md`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `README.md`; task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated read `README.md`; task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated edited `README.md`
- `TASK_WORKFLOW_EXAMPLE.md` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated read `TASK_WORKFLOW_EXAMPLE.md`; task scanless-workflow-ux checkpoint cp_20260616T075433Z_a03_validated edited `TASK_WORKFLOW_EXAMPLE.md`; task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated read `TASK_WORKFLOW_EXAMPLE.md`; task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated edited `TASK_WORKFLOW_EXAMPLE.md`
- `devspecs/tasks/scanless-workflow-ux/A02-document-tldr-first-launch-workflow-and-two-laye-plan.md` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task scanless-workflow-ux checkpoint cp_20260615T152127Z_a02_validated read `devspecs/tasks/scanless-workflow-ux/A02-document-tldr-first-launch-workflow-and-two-laye-plan.md`

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.

## Confidence Summary
- Primary file confidence: low
- Test coverage confidence: low
- Docs/config coverage confidence: low
- Git receipt confidence: low
- Noise risk: low
- Pack completeness: low

Why:
- no clear primary implementation file was found
- test companion coverage was not evident from the initial pack

Agent instruction:
Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.

## Suggested Starting Slice
Use `I01-rename-current-ds-map-orientation-flow-to-ds-rec-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `I01-rename-current-ds-map-orientation-flow-to-ds-rec-result.md` or `ds task checkpoint`.
