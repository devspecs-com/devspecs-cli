---
task_id: ds-task-freshness-evaluation-clean
slice: A03
kind: plan
stage: planned
decision: improve
created_at: 2026-06-04T08:51:55Z
---

# A03 Exclude Task Workspace Reads From Miss Metrics

## Goal
Make `ds task evaluate` measure implementation context, not the agent reading the task workspace.

## Description
Agents should read A00/A0* files, checkpoints, and `task.json`. Those reads are workflow resources. They should remain visible in raw observed evidence, but they should not count as hits, misses, critical-path recall, companion misses, or confidence mismatch.

The filter should be narrow: task workspace artifacts are metric-excluded; normal source/test/docs files still count.

## Resources
- `A00-index.md`
- `A03-exclude-task-workspace-reads-from-miss-metrics-result.md`
- `task.json`
- `internal/commands/task_evaluate.go`
- `internal/commands/task_test.go`

## Success Criteria
- [ ] A checkpoint that records `.devspecs/tasks/<task-id>/A00-index.md` as read does not produce a miss.
- [ ] A checkpoint that records `internal/commands/task.go` as read still counts normally.
- [ ] Raw `observed_context.files_read` still includes task workspace files for auditability.
- [ ] Explicit `--missed-file` task workspace artifacts are ignored for metrics, but normal explicit misses still count.
- [ ] Existing structured JSON evidence tests still pass.

## Tasks
- [ ] Add a helper that recognizes task workspace artifact paths in relative and absolute forms.
- [ ] Filter metric input paths before hit/miss, recall, companion-miss, and confidence-mismatch calculations.
- [ ] Preserve raw observed context unchanged in JSON output.
- [ ] Add focused tests for relative and absolute task workspace reads.
- [ ] Record an implementation checkpoint and update `A03-exclude-task-workspace-reads-from-miss-metrics-result.md`.

## Decision Gates
- Promote: metrics become cleaner without hiding audit evidence.
- Improve: workspace filtering works but path normalization needs broader coverage.
- Rework: checkpoint schema should distinguish resources from actual context before evaluation filtering.
- Rollback: filtering hides real implementation misses.
