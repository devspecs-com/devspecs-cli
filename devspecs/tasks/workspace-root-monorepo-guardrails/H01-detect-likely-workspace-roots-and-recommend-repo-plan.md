# Task workspace-root-monorepo-guardrails H01 Plan

## Goal
Detect likely workspace roots and recommend repo selection before expensive scans

## Description
Add a lightweight root-selection guard that detects when DevSpecs is probably being run at a workspace root instead of a single project repo. The first launchable behavior is a clear warning with candidate repo roots and next commands, not full workspace orchestration.

## Resources
- `H00-index.md`
- `H01-detect-likely-workspace-roots-and-recommend-repo-result.md`
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
- Repo/root detection helpers used by `scan`, `map`, `find`, and `task`.
- Auto-refresh/preflight paths that currently start traversal without explaining root choice.
- Human output and JSON output structs for root-selection warnings.
- Tests covering single repo, nested repo, and workspace root fixtures.

## Out-of-Scope Areas
- Scanning every child repo automatically.
- Full workspace config, cross-repo task workspaces, or team workspace semantics.
- Parallel traversal. That belongs in H03 after root detection is deterministic.
- Interactive prompts; the CLI should explain and return actionable next commands in non-interactive agent runs.

## Risks
- False positives could annoy normal monorepos; warnings must be precise and suppressible later if needed.
- False negatives still allow silent slow scans.
- JSON callers need structured warnings, not only stderr text.

## Success Criteria
- [ ] A fixture with multiple child repos/workspaces produces a clear "this looks like a workspace root" warning before expensive work.
- [ ] The warning suggests concrete candidate roots and a simple `cd <repo>` style next step.
- [ ] Single-repo behavior is unchanged.
- [ ] JSON output has structured root-warning metadata where relevant.
- [ ] Focused tests cover `scan`/auto-refresh plus at least one workflow command such as `map` or `task`.

## Tasks
- [ ] Locate current repo-root resolution and auto-refresh entry points.
- [ ] Define a conservative workspace-root heuristic: multiple repo markers, workspace manifests, or many high-level project roots.
- [ ] Add warning text and JSON metadata without changing the canonical default repo root for normal repos.
- [ ] Add fixtures/tests for common monorepo/workspace layouts.
- [ ] Run focused command tests and document any deferred false-positive handling.
- [ ] Update `H01-detect-likely-workspace-roots-and-recommend-repo-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: root warnings are clear, early, and low-noise in realistic fixtures.
- Improve: detection works but output needs tuning or more fixture coverage.
- Rework: heuristic is too noisy or misses the main dogfood failure.
- Rollback: warnings make normal repo workflows confusing.
