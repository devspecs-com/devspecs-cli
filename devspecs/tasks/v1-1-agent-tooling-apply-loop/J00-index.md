# Task v1-1-agent-tooling-apply-loop

## Task
v1.1 agent tooling init and apply loop surfaces

## Status
packed

## Series
J

## Profile
code-change

## Created At
2026-06-16T08:10:03Z

## Original Query
v1.1 agent tooling init and apply loop surfaces

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/v1-1-agent-tooling-apply-loop`

## Resources
- `task.json`
- `J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`
- `J01-extend-ds-init-with-interactive-tooling-selectio-result.md`
- `J02-generate-codex-cursor-claude-and-windsurf-slash-plan.md`
- `J02-generate-codex-cursor-claude-and-windsurf-slash-result.md`
- `J03-add-ds-apply-prompt-surface-for-next-or-explicit-plan.md`
- `J03-add-ds-apply-prompt-surface-for-next-or-explicit-result.md`
- `J04-add-slash-command-wrappers-for-ds-task-and-ds-ap-plan.md`
- `J04-add-slash-command-wrappers-for-ds-task-and-ds-ap-result.md`
- `J05-validate-apply-loop-respects-series-index-slice-plan.md`
- `J05-validate-apply-loop-respects-series-index-slice-result.md`

## Task Slices
- J01: Extend ds init with interactive tooling selection and background indexing. Plan: `J01-extend-ds-init-with-interactive-tooling-selectio-plan.md`. Result: `J01-extend-ds-init-with-interactive-tooling-selectio-result.md`.
- J02: Generate Codex Cursor Claude and Windsurf slash command or skill files. Plan: `J02-generate-codex-cursor-claude-and-windsurf-slash-plan.md`. Result: `J02-generate-codex-cursor-claude-and-windsurf-slash-result.md`.
- J03: Add ds apply prompt surface for next or explicit slice identifiers. Plan: `J03-add-ds-apply-prompt-surface-for-next-or-explicit-plan.md`. Result: `J03-add-ds-apply-prompt-surface-for-next-or-explicit-result.md`.
- J04: Add slash command wrappers for ds task and ds apply workflows. Plan: `J04-add-slash-command-wrappers-for-ds-task-and-ds-ap-plan.md`. Result: `J04-add-slash-command-wrappers-for-ds-task-and-ds-ap-result.md`.
- J05: Validate apply loop respects series index slice iteration and decision gates. Plan: `J05-validate-apply-loop-respects-series-index-slice-plan.md`. Result: `J05-validate-apply-loop-respects-series-index-slice-result.md`.

## Product Decisions
- `ds init` should become the friendly launch setup entry point: initialize local DevSpecs state, start indexing in the background where feasible, and offer a sleek tooling selector.
- Auto-detect and preselect likely agent surfaces where possible, but keep writes explicit and visible. Target Codex, Cursor, Claude, and Windsurf first.
- Generated agent files should make the next action obvious. The post-init success path should print one next step: `/ds:task "goal"`.
- `ds apply` should be prompt-only for v1.1. It resolves `next` or an explicit task/slice identifier and emits a bounded agent instruction for exactly one slice.
- Do not overbuild autonomous orchestration yet. Future agent process launchers should remain orthogonal to the CLI primitives and user-specific tooling.
- `ds apply` must preserve track fidelity: `M00` is the index, `M01`/`M02` are slices, `M01-1`/`M02-1` are improvement iterations, and every loop ends in a decision gate.

## Launch Contract
- Slash commands and skills are adapters over CLI primitives, not separate product logic.
- `/ds:task "goal"` should create or guide task-slice setup.
- `/ds:apply` should call the same apply prompt surface and constrain the agent to the current/next slice.
- Generated prompts must tell agents not to implement a whole track when the target is a slice.

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

- `internal/commands/init_test.go` - on-disk path matched task terms but was not in the indexed candidate set: init
- `internal/evalharness/agent_metrics_test.go` - on-disk path matched task terms but was not in the indexed candidate set: agent
- `internal/initflow/initflow_test.go` - on-disk path matched task terms but was not in the indexed candidate set: init

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/init_test.go` - on-disk path matched task terms but was not in the indexed candidate set: init; `internal/evalharness/agent_metrics_test.go` - on-disk path matched task terms but was not in the indexed candidate set: agent; `internal/initflow/initflow_test.go` - on-disk path matched task terms but was not in the indexed candidate set: init

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
Use `J01-extend-ds-init-with-interactive-tooling-selectio-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `J01-extend-ds-init-with-interactive-tooling-selectio-result.md` or `ds task checkpoint`.
