# Task v1-1-release-readiness-tag-gate

## Task
v1.1 release readiness and tag gate

## Status
packed

## Series
K

## Profile
code-change

## Created At
2026-06-16T08:11:38Z

## Original Query
v1.1 release readiness and tag gate

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/v1-1-release-readiness-tag-gate`

## Resources
- `task.json`
- `K01-run-launch-ready-cli-smoke-tests-across-task-rec-plan.md`
- `K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md`
- `K02-update-public-docs-and-tldr-for-v1-1-task-first-plan.md`
- `K02-update-public-docs-and-tldr-for-v1-1-task-first-result.md`
- `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-plan.md`
- `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-result.md`

## Task Slices
- K01: Run launch-ready CLI smoke tests across task recent find map init and apply. Plan: `K01-run-launch-ready-cli-smoke-tests-across-task-rec-plan.md`. Result: `K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md`.
- K02: Update public docs and tldr for v1.1 task-first launch story. Plan: `K02-update-public-docs-and-tldr-for-v1-1-task-first-plan.md`. Result: `K02-update-public-docs-and-tldr-for-v1-1-task-first-result.md`.
- K03: Cut v1.1.0 tag only after command surface and agent tooling gates pass. Plan: `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-plan.md`. Result: `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-result.md`.

## Release Decisions
- `v1.1.0` is the launch-ready tag target for the task-first command story.
- Do not tag `v1.1.0` until I/J implementation gates pass and public docs match the command surface.
- Smoke coverage must include `ds task`, `ds recent`, `ds find`, real/beta `ds map`, interactive `ds init` non-interactive fallbacks, and `ds apply` prompt output.
- Release notes should say `ds task` is the main workflow. `ds find` and `ds recent` are the diagnostic/evidence layer.
- If architecture mapping is still not stable, release it as `ds beta map` and document it as experimental.

## Gate Checklist
- [ ] `ds tldr` starts with task workflows and includes agent-oriented hotfix/epic/incident guidance.
- [ ] `ds map`/`ds recent` naming no longer confuses recent activity with architecture mapping.
- [ ] Agent tooling setup writes only selected files and prints one useful next step.
- [ ] `ds apply next` and `ds apply <identifier>` emit one-slice prompts with explicit decision gates.
- [ ] Generated result prompts ask what slice was attempted, which gate was tested, what changed, what evidence supports promote/improve, what remains, and what the next iteration should be.

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

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
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
Use `K01-run-launch-ready-cli-smoke-tests-across-task-rec-plan.md` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.

## Agent Preflight Checklist
- [ ] Verify the likely primary files against the repo before editing.
- [ ] Search for same-package or same-command tests if test confidence is not high.
- [ ] Check receipt-touched related files before assuming the pack is complete.
- [ ] Record files actually read, edited, tests run, misses, and noise in `K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md` or `ds task checkpoint`.
