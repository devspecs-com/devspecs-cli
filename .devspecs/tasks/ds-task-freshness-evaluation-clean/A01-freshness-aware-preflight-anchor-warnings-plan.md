---
task_id: ds-task-freshness-evaluation-clean
slice: A01
kind: plan
stage: planned
decision: improve
created_at: 2026-06-04T08:51:55Z
---

# A01 Freshness-Aware Preflight Anchor Warnings

## Goal
Make `ds task` warn when likely on-disk task anchors may be missing from the indexed/preflight candidate pool.

## Description
This slice should distinguish stale-index risk from retrieval-quality failure. It should not force a scan, change ranking, or create a second retrieval system.

The warning should be bounded and humble: "these repo files look like plausible anchors from disk, but they were not surfaced by the indexed preflight." It should expose risk, not assert relevance.

## Resources
- `A00-index.md`
- `A01-freshness-aware-preflight-anchor-warnings-result.md`
- `task.json`
- `internal/commands/task.go`
- `internal/commands/task_test.go`
- `internal/commands/refresh.go`

## Success Criteria
- [ ] A stale-index test can create a new relevant `task*.go` or `*_test.go` file after scan and see a warning when running `ds task --no-refresh`.
- [ ] Fresh-index behavior still passes existing task tests.
- [ ] Warning candidates are capped by count and reason length.
- [ ] Warnings exclude `.devspecs/tasks`, `_ignore`, fixtures, generated files, vendor, and oversized surfaces.
- [ ] Warnings do not claim the initial pack is definitely wrong.

## Tasks
- [ ] Define the narrow on-disk anchor heuristic for this slice: path/name/query matches, same-package tests, same-stem tests, and command filenames.
- [ ] Add a small `freshness_warnings` model to task manifest/output.
- [ ] Compare bounded on-disk anchors against predicted primary/test/docs/supporting paths.
- [ ] Render warnings in A00, JSON output, and human output.
- [ ] Add focused tests in `internal/commands/task_test.go`.
- [ ] Record an implementation checkpoint and update `A01-freshness-aware-preflight-anchor-warnings-result.md`.

## Decision Gates
- Promote: warnings separate stale-index risk from retrieval quality without bloating output.
- Improve: warning signal is useful but candidate selection is too noisy.
- Rework: warning code starts duplicating retrieval/ranking.
- Rollback: warnings create more false confidence or noise than they prevent.
