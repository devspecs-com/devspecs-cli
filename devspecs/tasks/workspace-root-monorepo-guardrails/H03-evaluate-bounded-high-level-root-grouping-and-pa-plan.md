# Task workspace-root-monorepo-guardrails H03 Plan

## Goal
Evaluate bounded high-level root grouping and parallel scan after deterministic root detection

## Description
Evaluate whether DevSpecs should parallelize by high-level roots after H01/H02 make root detection and traversal budgets deterministic. This is a decision/prototype slice, not a commitment to full workspace support.

## Resources
- `H00-index.md`
- `H03-evaluate-bounded-high-level-root-grouping-and-pa-result.md`
- `task.json`
- `internal/commands/task.go`
- `internal/commands/task_test.go`
- `README.md`

## Starting Context
### Files to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Tests to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Checkpoint Leads
Verify these prior checkpoint facts before widening search. They are not files the initial pack ranked as primary.
- `internal/commands/task.go` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `internal/commands/task.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `internal/commands/task.go`
- `internal/commands/task_test.go` [prior-test] - Verify this prior test lead before editing.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `internal/commands/task_test.go`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `internal/commands/task_test.go`
- `README.md` [prior-source] - Verify this prior source lead before choosing an edit target.
  Evidence: task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated read `README.md`; task task-freshness-sync-trust checkpoint cp_20260612T111812Z_b01_validated edited `README.md`

## Expected Change Surface
- Root-grouping heuristic from H01.
- Scan traversal scheduler and progress accounting from H02.
- Bench/fixture harness for synthetic multi-root workspaces.
- Docs language that keeps this framed as bounded support, not cloud/workspace orchestration.

## Out-of-Scope Areas
- Defaulting to full multi-repo indexing without a user-selected root.
- Cross-repo task workspaces or merged task packs.
- Persistent workspace graph semantics.
- Any parallelism that makes output nondeterministic or hides root warnings.

## Risks
- Faster wrong-root scans could make the UX look better while preserving the core mistake.
- Concurrent SQLite writes or noisy progress could reintroduce launch-stability issues.
- Users may infer full workspace support if the wording is too broad.

## Success Criteria
- [ ] A fixture or benchmark shows whether bounded high-level root grouping materially improves large workspace behavior.
- [ ] The result recommends one of: defer, prototype behind an experimental flag, or promote to default after more evidence.
- [ ] Any prototype preserves deterministic output order and SQLite write safety.
- [ ] Docs and help copy do not imply full workspace support.

## Tasks
- [ ] Review H01/H02 outcomes before starting.
- [ ] Build or reuse a multi-root fixture with heavy ignored directories.
- [ ] Compare sequential narrowed-root behavior against bounded grouped traversal.
- [ ] Decide whether parallel grouping belongs prelaunch, early patch, or later workspace support.
- [ ] Record the decision and any follow-up implementation slice.
- [ ] Update `H03-evaluate-bounded-high-level-root-grouping-and-pa-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: bounded grouping is measurably helpful and does not blur root selection.
- Improve: promising, but needs stronger fixture coverage or concurrency hardening.
- Rework: grouping should become explicit workspace support instead of hidden scan behavior.
- Rollback: detection/warnings solve enough and parallelism adds more risk than value.
