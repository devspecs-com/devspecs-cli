# Task task-freshness-sync-trust

## Task
Task freshness and sync trust repair

## Status
packed

## Series
B

## Profile
greenfield

## Created At
2026-06-12T07:39:45Z

## Original Query
Task freshness and sync trust repair

## Track Intent
This is the immediate trust-repair track for task artifact freshness. It exists because dogfooding exposed a confusing state: a user manually corrected a `Q00` index after `sync`, then `ds task status` reported `Q00` as stale even though `task.json` still had the correct slice state.

The product problem is not just stale detection. It is that users cannot tell whether they should preserve a readability improvement, let the CLI own the generated index, or run a clean refresh mode that updates captured corpus state without rewriting authored task docs.

## Timing
Immediate / patch-level. This should land before we ask more teams to rely on task workspaces for handoff discipline.

## Product Decisions
- New user task workspaces should continue to default to `devspecs/tasks`.
- Legacy `.devspecs/tasks` auto-detection should remain for internal dogfood repos and explicit local workspaces.
- `task.json` lifecycle state and captured artifact freshness should be reported as separate concepts.
- Refresh UX must not require users to sacrifice readable authored sections just to make warnings go away.

## Non-Goals
- Do not reintroduce full generated index rewrites during lifecycle commands.
- Do not make `ds scan` silently own task docs.
- Do not treat every manual edit under `devspecs/tasks` as corruption.

## Decision Gates
- Promote to implementation when the CLI behavior can be stated as a small set of commands and warning states.
- Improve if the naming is unclear (`sync`, `refresh`, `recapture`, or another term).
- Rework if the design requires overwriting authored Markdown.
- Roll back any implementation that makes task status less trustworthy.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/task-freshness-sync-trust`

## Resources
- `task.json`
- `B01-add-explicit-task-refresh-mode-that-updates-inde-plan.md`
- `B01-add-explicit-task-refresh-mode-that-updates-inde-result.md`
- `B02-clarify-stale-warnings-so-task-json-state-and-ca-plan.md`
- `B02-clarify-stale-warnings-so-task-json-state-and-ca-result.md`
- `B03-preserve-manual-readability-edits-while-letting-plan.md`
- `B03-preserve-manual-readability-edits-while-letting-result.md`
- `B04-explain-sandboxed-database-open-failures-with-ap-plan.md`
- `B04-explain-sandboxed-database-open-failures-with-ap-result.md`

## Task Slices
- B01: Add explicit task refresh mode that updates index freshness without rewriting authored task docs. Plan: `B01-add-explicit-task-refresh-mode-that-updates-inde-plan.md`. Result: `B01-add-explicit-task-refresh-mode-that-updates-inde-result.md`.
- B02: Clarify stale warnings so task.json state and captured artifact freshness are reported separately. Plan: `B02-clarify-stale-warnings-so-task-json-state-and-ca-plan.md`. Result: `B02-clarify-stale-warnings-so-task-json-state-and-ca-result.md`.
- B03: Preserve manual readability edits while letting the CLI refresh captured corpus state. Plan: `B03-preserve-manual-readability-edits-while-letting-plan.md`. Result: `B03-preserve-manual-readability-edits-while-letting-result.md`.
- B04: Explain sandboxed database-open failures with approval and DEVSPECS_HOME guidance. Plan: `B04-explain-sandboxed-database-open-failures-with-ap-plan.md`. Result: `B04-explain-sandboxed-database-open-failures-with-ap-result.md`.

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

- `internal/commands/freshness_test.go` - on-disk path matched task terms but was not in the indexed candidate set: freshness
- `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task
- `internal/freshness/freshness_test.go` - on-disk path matched task terms but was not in the indexed candidate set: freshness
- `internal/store/task_checkpoint_facts_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/freshness_test.go` - on-disk path matched task terms but was not in the indexed candidate set: freshness; `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task; `internal/freshness/freshness_test.go` - on-disk path matched task terms but was not in the indexed candidate set: freshness; `internal/store/task_checkpoint_facts_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before committing to implementation scope.

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
Use the evidence to define the first bounded planning artifact, evaluation signal, and next-slice decision before implementation scope expands.

## Suggested Starting Slice
Use `B01-add-explicit-task-refresh-mode-that-updates-inde-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `B01-add-explicit-task-refresh-mode-that-updates-inde-result.md` or `ds task checkpoint`.
