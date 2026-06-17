# Task v1-1-release-readiness-tag-gate K01 Result

## Summary
- Target: `K01` - Run launch-ready CLI smoke tests across task recent find map init and apply
- Outcome: Fresh-repo smoke passed functionally across `init`, `map`, `recent`, `find`, `task`, `prompt`, `apply`, and checkpoint progression, but the release gate should iterate before tag work because smoke surfaced launch-surface papercuts.

## Changed Files
- `devspecs/tasks/v1-1-release-readiness-tag-gate/K00-index.md`
- `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-release-smoke-transcript.md`
- `devspecs/tasks/task-list-inventory-ux/M00-index.md`
- `devspecs/tasks/task-list-inventory-ux/M01-define-task-list-open-plan-inventory-ux-and-rout-plan.md`
- `devspecs/tasks/task-list-inventory-ux/*`

## Tests
- Fresh repo smoke with repo-local `DEVSPECS_HOME`: `ds init --yes --tool all --index foreground`
- Smoke: `ds map`, `ds map --json`, `ds recent`, `ds find "token refresh retry" --json`
- Smoke: `ds task --id launch-smoke ... --json`, `ds task prompt launch-smoke --json`, `ds apply launch-smoke --json`
- Smoke: `ds task checkpoint launch-smoke --target A01 --stage validated --decision promote ... --json`, then `ds apply launch-smoke --json` selected `A02`
- Agent tooling file smoke: all 8 generated Codex/Cursor/Claude/Windsurf files existed
- External agent CLI smoke: `codex --help` blocked with Windows App `Access is denied`; `claude --help` worked; constrained Claude `/ds-task` stdin probe was partial only

## Decision
- Improve. Core launch flow works, but do not move straight to K02/K03 until targeted follow-up repairs run and K01-1 repeats the smoke.

## Follow-up
- Follow the recorded route: `K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03`.
- Planned `M` track for `ds task list` open-plan inventory and retiring the old top-level artifact `ds list` from the public launch surface.
- Treat PowerShell stderr/progress capture, sparse-docs init suggestion noise, and external slash-command smoke limits as K01 findings.

## References
- `K00-index.md`
- `K01-run-launch-ready-cli-smoke-tests-across-task-rec-plan.md`
- `K01-release-smoke-transcript.md`

## Checkpoints
- Use `ds task checkpoint v1-1-release-readiness-tag-gate --target K01` to append structured evidence.

### Checkpoint
- Created At: 2026-06-17T12:19:18Z
- Stage: validated
- Decision: improve
- Source: `checkpoints/20260617-121918-validated.md`
- Structured Evidence: `checkpoints/20260617-121918-validated.json`
- Note: Follow release route K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03 before tag work.
- What changed: Fresh-repo launch smoke passed core CLI workflows, but surfaced launch-surface papercuts: top-level ds list still appears in public help, PowerShell combined stderr/stdout capture can make task progress look like an error, init sparse-docs suggestion is noisy for tiny valid repos, and external slash command execution is only partially smokeable here.
- Evidence for decision: 2 file(s) read; 5 file(s) edited; 3 test command(s); 1 missed file(s); 1 noise file(s)
- What remains: next target B03; next decision improve; resolve missed files
- Next iteration: B03 with decision improve
- Files read:
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-run-launch-ready-cli-smoke-tests-across-task-rec-plan.md`
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K00-index.md`
- Files edited:
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K00-index.md`
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-run-launch-ready-cli-smoke-tests-across-task-rec-result.md`
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-release-smoke-transcript.md`
  - `devspecs/tasks/task-list-inventory-ux/M00-index.md`
  - `devspecs/tasks/task-list-inventory-ux/M01-define-task-list-open-plan-inventory-ux-and-rout-plan.md`
- Tests run:
  - `K01 fresh repo smoke: ds init/map/recent/find/task/prompt/apply/checkpoint/apply`
  - `Agent tooling file smoke: generated 8/8 Codex Cursor Claude Windsurf files`
  - `External CLI smoke: codex help blocked by Access is denied; claude help succeeded; claude /ds-task stdin probe partial`
- Missed files:
  - `top-level ds list remains in public help`
- Noise files:
  - `ds task progress on stderr appears as NativeCommandError when PowerShell captures combined streams`
