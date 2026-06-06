# Task ds-task-workflow-ux-boilerplate

## Task
Capture preferred ds task workflow UX for series, slices, iterations, lifecycle decisions, and learnings

## Status
packed

## Created At
2026-06-04T14:06:50Z

## Original Query
Capture preferred ds task workflow UX for series, slices, iterations, lifecycle decisions, and learnings

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.devspecs/raw-output-samples/generated-a-series/ds-task-workflow-ux-boilerplate`

## Resources
- `task.json`
- `A01-define-series-and-slice-lifecycle-model-plan.md`
- `A01-define-series-and-slice-lifecycle-model-result.md`
- `A02-scaffold-iteration-plan-artifacts-plan.md`
- `A02-scaffold-iteration-plan-artifacts-result.md`
- `A03-record-completion-supersedence-split-and-rollbac-plan.md`
- `A03-record-completion-supersedence-split-and-rollbac-result.md`
- `A04-surface-learnings-as-bounded-checkpoint-evidence-plan.md`
- `A04-surface-learnings-as-bounded-checkpoint-evidence-result.md`

## Task Slices
- A01: define series and slice lifecycle model. Plan: `A01-define-series-and-slice-lifecycle-model-plan.md`. Result: `A01-define-series-and-slice-lifecycle-model-result.md`.
- A02: scaffold iteration-plan artifacts. Plan: `A02-scaffold-iteration-plan-artifacts-plan.md`. Result: `A02-scaffold-iteration-plan-artifacts-result.md`.
- A03: record completion supersedence split and rollback decisions. Plan: `A03-record-completion-supersedence-split-and-rollbac-plan.md`. Result: `A03-record-completion-supersedence-split-and-rollbac-result.md`.
- A04: surface learnings as bounded checkpoint evidence. Plan: `A04-surface-learnings-as-bounded-checkpoint-evidence-plan.md`. Result: `A04-surface-learnings-as-bounded-checkpoint-evidence-result.md`.

## Relevant Map Areas
- `internal/commands`
- `internal/retrieval`

## Likely Primary Files
- `internal/commands/task.go` - internal/commands/task.go (go)
  Evidence: anchor-first ranking: score 24.000; matches capture, task, decisions; fields body, path, title; query term match in path: task; query term match in body: slices
- `internal/commands/task_evaluate.go` - internal/commands/task_evaluate.go (go)
  Evidence: anchor-first ranking: score 22.861; matches task; fields path, title, body; query term match in path: task; query term match in body: slices
- `internal/commands/map.go` - internal/commands/map.go (go)
  Evidence: query term match in body: task; query term match in body: workflow; query term match in body: slices
- `internal/commands/capture.go` - internal/commands/capture.go (go)
  Evidence: anchor-first ranking: score 24.000; matches capture; fields path, title, body; query term match in path: capture
- `internal/retrieval/retrieval.go` - internal/retrieval/retrieval.go (go)
  Evidence: query term match in body: task; query term match in body: workflow; query term match in body: lifecycle

## Likely Tests
None found in the initial preflight.

## Likely Docs / Plans / Config
None found in the initial preflight.

## Supporting Context
None found in the initial preflight.

## Related Git Receipts
- `4af4d35` 2026-06-04 - feat: author map handoffs from git task signals
  Matched paths: `internal/commands/map.go`
- `94997de` 2026-06-02 - feat: repair map onboarding anchors
  Matched paths: `internal/commands/map.go`, `internal/retrieval/retrieval.go`
- `a4ebf74` 2026-06-01 - feat: repair ds map handoff trust
  Matched paths: `internal/commands/map.go`, `internal/retrieval/retrieval.go`

## Noise Risks
None found in the initial preflight.

## Freshness Warnings
These on-disk paths look task-related but were not present in the indexed candidate set. Treat them as stale-index risk, not proof that the initial pack is wrong.

- `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task

## Known Knowns
- The preflight found likely primary implementation files.
- Git receipts provide historical trust evidence for packed paths.

## Known Unknowns
- Relevant tests may be missing from the initial pack.
- On-disk task anchors may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.

## Confidence Summary
- Primary file confidence: high
- Test coverage confidence: low
- Docs/config coverage confidence: low
- Git receipt confidence: high
- Noise risk: low
- Pack completeness: low

Why:
- found 5 likely primary file(s)
- test companion coverage was not evident from the initial pack
- found 3 related Git receipt(s)

Agent instruction:
Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.

## Suggested Starting Slice
Use `A01-define-series-and-slice-lifecycle-model-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `A01-define-series-and-slice-lifecycle-model-result.md` or `ds task checkpoint`.
