---
task_id: ds-task-freshness-evaluation-clean
slice: A03
kind: result
stage: implemented
decision: promote
created_at: 2026-06-04T08:51:55Z
updated_at: 2026-06-04T12:14:01Z
---

# A03 Exclude Task Workspace Reads From Miss Metrics Result

## What Was Attempted
- Added metric-only filtering for task workspace artifact paths in `ds task evaluate`.
- Preserved raw `observed_context` while filtering paths before hits, misses, recall, companion misses, receipt misses, and confidence mismatch.
- Added focused coverage for relative and absolute task workspace reads plus explicit task-artifact missed files.

## Files Actually Read
- `internal/commands/task_evaluate.go`
- `internal/commands/task_test.go`

## Files Actually Edited
- `internal/commands/task_evaluate.go`
- `internal/commands/task_test.go`
- `.devspecs/tasks/ds-task-freshness-evaluation-clean/A03-exclude-task-workspace-reads-from-miss-metrics-result.md`

## Tests Actually Read
- `internal/commands/task_test.go`

## Tests Actually Run
- `go test ./internal/commands -run TestTask -count=1`
- `go test ./internal/repo -count=1`
- `go test ./cmd/ds -count=1`
- `go build -o ds.exe ./cmd/ds`
- `ds task evaluate ds-task-freshness-evaluation-clean --json`

## Critical Files DevSpecs Missed
- None for this implementation slice.

## Distracting Files DevSpecs Included
- None for this implementation slice.

## Outcome
- Implemented. Task workspace paths remain visible in raw observed evidence but no longer count as evaluated misses or metric denominator input.
- Normal implementation hits and normal explicit missed files still count.
- Live dogfood evaluation still reports the baseline `B` result from the existing planned checkpoint; final workflow usefulness remains an A04 decision.

## Decision
- Promote A03. The filter is narrow and preserves auditability.

## Next Recommended Slice
- A04: dogfood checkpoint/evaluate friction.

## Checkpoint Notes
- No CLI checkpoint was created for A03 because checkpoint frontmatter and slice-targeting cleanup is still tracked in A04.

### Checkpoint
- Created At: 2026-06-04T12:21:49Z
- Stage: implemented
- Decision: promote
- Source: `checkpoints/20260604-122149-implemented.md`
- Structured Evidence: `checkpoints/20260604-122149-implemented.json`
- Note: A03 promoted: task workspace artifacts no longer pollute miss metrics.
- Files read:
  - `internal/commands/task_evaluate.go`
  - `internal/commands/task_test.go`
- Files edited:
  - `internal/commands/task_evaluate.go`
  - `internal/commands/task_test.go`
- Tests run:
  - `go test ./internal/commands -run TestTask -count=1`
  - `go test ./internal/repo -count=1`
  - `go test ./cmd/ds -count=1`
  - `go build -o ds.exe ./cmd/ds`
