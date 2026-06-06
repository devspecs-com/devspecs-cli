---
task_id: ds-task-freshness-evaluation-clean
kind: task-series-index
stage: completed
decision: promote
created_at: 2026-06-04T08:51:55Z
updated_at: 2026-06-04T12:22:18Z
source: ds task
---

# DS Task Freshness/Evaluation Clean Series

## Goal
Make `ds task` safer to trust and cleaner to evaluate without changing general retrieval scoring, dynamic suppression, or default test inclusion behavior.

## Description
This series comes from the `ds task` dogfood runs. The key learning is that we need to separate stale-index failures, checkout/root failures, and evaluation bookkeeping failures before optimizing ranking or suppression.

The work should stay small and evidence-oriented:

- warn when obvious on-disk anchors may be missing from the indexed candidate pool;
- resolve Git worktree roots correctly;
- exclude task workspace reads from evaluation metrics while preserving raw observed evidence;
- record dogfood friction before deciding whether to promote, improve, or rework the workflow.

## Resources
- `task.json`
- `A01-freshness-aware-preflight-anchor-warnings-plan.md`
- `A01-freshness-aware-preflight-anchor-warnings-result.md`
- `A02-git-worktree-root-detection-plan.md`
- `A02-git-worktree-root-detection-result.md`
- `A03-exclude-task-workspace-reads-from-miss-metrics-plan.md`
- `A03-exclude-task-workspace-reads-from-miss-metrics-result.md`
- `A04-dogfood-checkpoint-and-evaluate-friction-plan.md`
- `A04-dogfood-checkpoint-and-evaluate-friction-result.md`
- `agent-handoff-report.md`
- `checkpoints/20260604-085549-planned.md`
- `checkpoints/20260604-085549-planned.json`

## Slices
- A01: freshness-aware preflight anchor warnings. Decision: promote.
- A02: Git worktree root detection. Decision: promote.
- A03: exclude task workspace reads from miss metrics. Decision: promote.
- A04: dogfood checkpoint/evaluate friction. Decision: promote.

## Baseline Evidence
- Fresh-index `ds task` found `internal/commands/task.go` and `internal/commands/task_evaluate.go`.
- Baseline evaluation reported usefulness `B`, primary file hit true, and critical-path recall `2/5`.
- Important misses were `internal/commands/task_test.go`, `internal/repo/repo.go`, and `internal/repo/repo_test.go`.
- Checkpoint markdown currently puts `stage`, `decision`, and `created_at` in body sections; future checkpoint markdown should put those fields in frontmatter.
- `ds task checkpoint` currently appends to the first slice result by default, which is awkward for whole-series planning and later slices.

## Final Evidence
- Checkpoint markdown now keeps lifecycle metadata in frontmatter.
- `ds task checkpoint --slice <slice>` now appends to the selected slice result.
- A01-A04 each have structured checkpoints.
- Final `ds task evaluate ds-task-freshness-evaluation-clean --json` usefulness class: B.
- Final critical-path recall: `2/5`.
- Final primary file hit: true.
- Remaining misses are `internal/commands/task_test.go`, `internal/repo/repo.go`, and `internal/repo/repo_test.go`; these are real support/test-file recall issues, not A03 metric filtering failures.
- No A03.1 is needed because A03 passed its promote gate.
- Consolidated next-agent guidance is captured in `agent-handoff-report.md`.

## Success Criteria
- [x] `ds task` can warn about likely stale/missing on-disk anchors without claiming the pack is wrong.
- [x] Git worktrees resolve to the active checkout root for task start, checkpoint, and evaluate.
- [x] `ds task evaluate` excludes task workspace reads from metric calculations while preserving them in observed evidence.
- [x] Each implementation slice produces a structured checkpoint and a useful result note.
- [x] The final dogfood assessment clearly chooses promote, improve, rework, or rollback.

## Tasks
- [x] Implement A01 and checkpoint with actual files/tests/misses/noise.
- [x] Implement A02 and checkpoint with actual files/tests/misses/noise.
- [x] Implement A03 and checkpoint with actual files/tests/misses/noise.
- [x] Run `ds task evaluate ds-task-freshness-evaluation-clean --json` after A03.
- [x] Complete A04 with friction points and the final decision.

## Decision Gates
- Promote: the workflow materially improves task starts and produces clean evaluation evidence.
- Improve: the workflow helps, but freshness/checkpoint/evaluate UX needs small guardrails.
- Rework: the workspace structure feels heavier than the implementation task.
- Rollback: the workflow creates false confidence, noisy index substrate, or misleading metrics.
