---
task_id: ds-task-freshness-evaluation-clean
slice: A04
kind: plan
stage: planned
decision: improve
created_at: 2026-06-04T08:51:55Z
---

# A04 Dogfood Checkpoint And Evaluate Friction

## Goal
Use this series to decide whether the `ds task` workflow is helping implementation or adding planning overhead.

## Description
This slice is the feedback loop for A01-A03. It should capture actual files read/edited, tests run, misses/noise, evaluation output, and UX friction.

This is also where we track two checkpoint UX issues found while creating the series:

- checkpoint markdown should put `stage`, `decision`, and `created_at` in frontmatter, not body sections;
- checkpointing needs explicit slice targeting or a better whole-series target, instead of always appending to the first slice result.

## Resources
- `A00-index.md`
- `A01-freshness-aware-preflight-anchor-warnings-result.md`
- `A02-git-worktree-root-detection-result.md`
- `A03-exclude-task-workspace-reads-from-miss-metrics-result.md`
- `A04-dogfood-checkpoint-and-evaluate-friction-result.md`
- `task.json`
- `checkpoints/20260604-085549-planned.md`
- `checkpoints/20260604-085549-planned.json`
- `internal/commands/task.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/task_test.go`

## Success Criteria
- [ ] Each implementation slice has at least one structured checkpoint.
- [ ] `ds task evaluate ds-task-freshness-evaluation-clean --json` runs after A03.
- [ ] Result files record critical files found, missed, noisy inclusions, tests run, and decision.
- [ ] A04 result records concrete friction points and a final usefulness class.
- [ ] Frontmatter and slice-targeting follow-ups are either implemented or explicitly deferred.

## Tasks
- [ ] After A01, checkpoint with actual files read/edited, tests run, misses/noise, and `--git-diff` if useful.
- [ ] After A02, checkpoint with actual files read/edited, tests run, misses/noise, and `--git-diff` if useful.
- [ ] After A03, checkpoint and run `ds task evaluate ds-task-freshness-evaluation-clean --json`.
- [ ] Verify that task workspace reads no longer pollute miss metrics after A03.
- [ ] Decide whether checkpoint frontmatter and explicit slice targeting are part of this series or a follow-up.
- [ ] Complete `A04-dogfood-checkpoint-and-evaluate-friction-result.md`.

## Decision Gates
- Promote: `ds task` made the next agent faster and produced clean evaluation evidence.
- Improve: the structure helped, but checkpoint/evaluate UX needs small guardrails.
- Rework: slice/result/checkpoint structure feels heavier than the task.
- Rollback: the workflow encouraged false confidence or polluted the index/evaluation.
