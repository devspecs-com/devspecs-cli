# Task task-list-inventory-ux

## Task
Task list inventory UX: list open plans by default, support all closed since filters, and remove or retire old artifact list from public launch surface

## Status
packed

## Series
M

## Profile
code-change

## Created At
2026-06-17T12:08:11Z

## Original Query
Task list inventory UX: list open plans by default, support all closed since filters, and remove or retire old artifact list from public launch surface

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/task-list-inventory-ux`

## Resources
- `task.json`
- `M01-define-task-list-open-plan-inventory-ux-and-rout-plan.md`
- `M01-define-task-list-open-plan-inventory-ux-and-rout-result.md`
- `M02-implement-ds-task-list-default-open-output-with-plan.md`
- `M02-implement-ds-task-list-default-open-output-with-result.md`
- `M03-retire-old-top-level-ds-list-from-public-surface-plan.md`
- `M03-retire-old-top-level-ds-list-from-public-surface-result.md`

## Task Slices
- M01: Define task list open-plan inventory UX and route semantics. Plan: `M01-define-task-list-open-plan-inventory-ux-and-rout-plan.md`. Result: `M01-define-task-list-open-plan-inventory-ux-and-rout-result.md`.
- M02: Implement ds task list default open output with --all --closed and --since filters. Plan: `M02-implement-ds-task-list-default-open-output-with-plan.md`. Result: `M02-implement-ds-task-list-default-open-output-with-result.md`.
- M03: Retire old top-level ds list from public surface or replace it with task inventory guidance. Plan: `M03-retire-old-top-level-ds-list-from-public-surface-plan.md`. Result: `M03-retire-old-top-level-ds-list-from-public-surface-result.md`.

## Product Read
The current top-level `ds list` is an artifact inventory command. It is too broad for launch onboarding and does not answer the question users and agents keep asking: "what plans or task tracks are still open, and what should I do next?"

`ds task list` should become the plan/work inventory command:
- Default output lists open task tracks and their next open target.
- `--all` includes closed tracks.
- `--closed` shows only completed/terminal tracks.
- `--since <duration|date>` filters by task or slice update time.
- JSON output should be stable enough for agents to route work.
- Human output should explain the track intent, not just print filenames.

## Cross-Track Loop Note
The release route `K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03` shows that DevSpecs also needs a lightweight way to represent a cross-track route. Do not build a full orchestration engine in this track, but make `ds task list` leave room for a later route view.

## Launch Surface Decision
The old top-level `ds list` should not remain a prominent public launch command unless it is renamed or hidden as an artifact-level diagnostic. It competes with the task-first story and is less useful than `ds find`, `ds recent`, and the proposed `ds task list`.

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

- `internal/adapters/openspec/openspec_test.go` - on-disk path matched task terms but was not in the indexed candidate set: open
- `internal/commands/golden_test.go` - on-disk path matched task terms but was not in the indexed candidate set: old
- `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task
- `internal/openspecmetrics/metrics_test.go` - on-disk path matched task terms but was not in the indexed candidate set: open
- `internal/store/task_checkpoint_facts_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task
- `.private/strategy/docs/plans/2026-05-19-first-index-public-eval-plan.md` - on-disk path matched task terms but was not in the indexed candidate set: plans, public
- `.private/strategy/docs/plans/2026-05-19-mined-holdout-crosscheck-plan.md` - on-disk path matched task terms but was not in the indexed candidate set: plans, old
- `.private/strategy/docs/plans/2026-05-20-overfit-resistant-public-eval-plan.md` - on-disk path matched task terms but was not in the indexed candidate set: plans, public

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/adapters/openspec/openspec_test.go` - on-disk path matched task terms but was not in the indexed candidate set: open; `internal/commands/golden_test.go` - on-disk path matched task terms but was not in the indexed candidate set: old; `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task; `internal/openspecmetrics/metrics_test.go` - on-disk path matched task terms but was not in the indexed candidate set: open

## Checkpoint Leads
The current pack is weak, so these compact checkpoint facts are verification leads only. They are not pack-ranked edit targets.

- `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `devspecs/tasks/v1-1-agent-tooling-apply-loop/J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`
- `internal/commands/init_test.go` [prior-test, checkpoint_fact]
  Agent check: Verify this prior test lead before editing.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/init_test.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated edited `internal/commands/init_test.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read test `internal/commands/init_test.go`
- `internal/commands/init.go` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/init.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated edited `internal/commands/init.go`; task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated learned: Do not print /ds:task until J02 actually writes slash-command files; J01 should keep the next step as valid CLI: ds task goal.
- `internal/initflow/initflow.go` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/initflow/initflow.go`
- `internal/commands/scan.go` [prior-source, checkpoint_fact]
  Agent check: Verify this prior source lead before choosing an edit target.
  Evidence: task v1-1-agent-tooling-apply-loop checkpoint cp_20260616T141106Z_j01_validated read `internal/commands/scan.go`; task task-freshness-sync-trust checkpoint cp_20260615T185505Z_b04_validated read `internal/commands/scan.go`; task task-freshness-sync-trust checkpoint cp_20260615T185505Z_b04_validated edited `internal/commands/scan.go`

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before editing.

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
Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.

## Suggested Starting Slice
Use `M01-define-task-list-open-plan-inventory-ux-and-rout-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `M01-define-task-list-open-plan-inventory-ux-and-rout-result.md` or `ds task checkpoint`.
