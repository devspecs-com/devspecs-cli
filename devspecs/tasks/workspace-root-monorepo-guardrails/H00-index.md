# Task workspace-root-monorepo-guardrails

## Dogfood Trigger
In a client workspace, an agent ran DevSpecs from the workspace root instead of a single repo and hit long-running commands/timeouts without a useful explanation. This is a launch trust issue: users should not have to understand DevSpecs internals to know that they picked the wrong root.

## Product Stance
Do not take on full workspace support as the first fix. Prelaunch behavior should detect likely workspace roots, explain the risk, recommend concrete repo roots, and make long scans visibly bounded. Parallel high-level root scanning is promising, but only after root detection and ignore/budget behavior are deterministic.

## Non-Goals
- Do not silently scan every child repo in a workspace root.
- Do not market this as full monorepo/workspace support yet.
- Do not make `node_modules`, build outputs, vendored deps, or generated artifacts candidates for expensive traversal.
- Do not hide timeouts behind empty output.

## Task
Workspace root and monorepo guardrails for scan/map/find/task commands

## Status
packed

## Series
H

## Profile
code-change

## Created At
2026-06-15T15:37:17Z

## Original Query
Workspace root and monorepo guardrails for scan/map/find/task commands

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/workspace-root-monorepo-guardrails`

## Resources
- `task.json`
- `H01-detect-likely-workspace-roots-and-recommend-repo-plan.md`
- `H01-detect-likely-workspace-roots-and-recommend-repo-result.md`
- `H02-add-traversal-budgets-progress-output-and-ignore-plan.md`
- `H02-add-traversal-budgets-progress-output-and-ignore-result.md`
- `H03-evaluate-bounded-high-level-root-grouping-and-pa-plan.md`
- `H03-evaluate-bounded-high-level-root-grouping-and-pa-result.md`

## Task Slices
- H01: Detect likely workspace roots and recommend repo selection before expensive scans. Plan: `H01-detect-likely-workspace-roots-and-recommend-repo-plan.md`. Result: `H01-detect-likely-workspace-roots-and-recommend-repo-result.md`.
- H02: Add traversal budgets, progress output, and ignored-directory explanations for scan map find and task. Plan: `H02-add-traversal-budgets-progress-output-and-ignore-plan.md`. Result: `H02-add-traversal-budgets-progress-output-and-ignore-result.md`.
- H03: Evaluate bounded high-level root grouping and parallel scan after deterministic root detection. Plan: `H03-evaluate-bounded-high-level-root-grouping-and-pa-plan.md`. Result: `H03-evaluate-bounded-high-level-root-grouping-and-pa-result.md`.

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

- `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_pack_scout_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_pack_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_source_manifest_consumption_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_source_manifest_recovery_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands
- `internal/commands/find_source_pack_mode_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands; `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands; `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands; `internal/commands/find_pack_scout_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find, commands

## Checkpoint Leads
The current pack is weak, so these compact checkpoint facts are verification leads only. They are not pack-ranked edit targets.

- `internal/commands/task.go` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `internal/commands/task.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `internal/commands/task.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated learned: Use ds task refresh as the visible command for non-destructive task artifact recapture; keep sync available but stop making stale warnings point users at sync.
- `internal/commands/task_test.go` [prior-test, checkpoint_fact]
  Agent check: Verify this prior test lead before editing.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `internal/commands/task_test.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `internal/commands/task_test.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read test `internal/commands/task_test.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated learned: Use ds task refresh as the visible command for non-destructive task artifact recapture; keep sync available but stop making stale warnings point users at sync.
- `README.md` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `README.md`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `README.md`

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
Use `H01-detect-likely-workspace-roots-and-recommend-repo-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `H01-detect-likely-workspace-roots-and-recommend-repo-result.md` or `ds task checkpoint`.
