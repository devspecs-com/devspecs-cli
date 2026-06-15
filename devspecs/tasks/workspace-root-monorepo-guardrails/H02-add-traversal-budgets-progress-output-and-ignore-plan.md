# Task workspace-root-monorepo-guardrails H02 Plan

## Goal
Add traversal budgets, progress output, and ignored-directory explanations for scan map find and task

## Description
Make expensive traversal feel bounded and explainable. When DevSpecs scans or auto-refreshes, users and agents should see progress, skipped heavy directories, and a useful failure message before a sandbox/IDE timeout turns into mystery.

## Resources
- `H00-index.md`
- `H02-add-traversal-budgets-progress-output-and-ignore-result.md`
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
- Scan traversal and ignore handling.
- Auto-refresh callers used by `map`, `find`, and `task`.
- Human stderr progress and JSON diagnostics.
- Tests for ignored directories, traversal budgets, and timeout-adjacent errors.

## Out-of-Scope Areas
- Full workspace support or cross-repo indexing semantics.
- Background daemons/watchers.
- Scanning dependency/build directories for intent artifacts.
- Parallel root scanning; defer to H03.

## Risks
- Progress output can break JSON callers if routed to stdout instead of stderr.
- Budgets that are too low can make real repos look broken.
- Ignore explanations can become noisy if printed on every tiny repo.

## Success Criteria
- [ ] `node_modules`, `.git`, build outputs, vendored deps, and generated-heavy dirs are skipped consistently or explained by existing config.
- [ ] Long scans emit bounded progress to stderr without corrupting JSON output.
- [ ] Budget/timeout-adjacent failures say what was being scanned and how to narrow the root.
- [ ] Auto-refresh callers inherit the same progress/error behavior.
- [ ] Tests cover progress routing, ignored-directory behavior, and actionable error copy.

## Tasks
- [ ] Audit current scan ignore defaults and auto-refresh traversal path.
- [ ] Define launch-safe heavy-directory defaults and any budget thresholds.
- [ ] Route progress to stderr and structured diagnostics to JSON metadata.
- [ ] Add tests that a large ignored tree does not dominate scan time or output.
- [ ] Update docs if users need to understand skipped directories.
- [ ] Update `H02-add-traversal-budgets-progress-output-and-ignore-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: long/large-root behavior is explainable and bounded without changing normal repo output too much.
- Improve: progress works but copy/budgets need more polish.
- Rework: budget or ignore semantics risk hiding legitimate intent artifacts.
- Rollback: output noise or JSON breakage outweighs the reliability gain.
