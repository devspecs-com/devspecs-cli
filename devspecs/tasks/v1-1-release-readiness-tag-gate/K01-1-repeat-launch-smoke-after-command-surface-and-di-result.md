# Task v1-1-release-readiness-tag-gate K01-1 Result

## Summary
- Target: `K01-1` - Repeat launch smoke after command surface and diagnostic ranking repairs
- Outcome: Passed. The launch flow is ready to promote into K02 docs/tldr and K03 tag-gate work.

## Completion Contract
- Attempted slice: `K01-1` - Repeat launch smoke after command surface and diagnostic ranking repairs
- Gate tested: promote
- What changed: added a K01-1 iteration artifact and recorded the updated smoke evidence.
- Evidence for decision: fresh isolated smoke repo completed 16 CLI steps with all exits `0`.
- What remains: K02 public docs/tldr pass, then K03 final tag gate.
- Next iteration: promote to K02.

## Smoke Evidence
- Binary: `ds dev (commit: 87d8731, built: 2026-06-17T13:40:54Z)`
- Fixture: `C:\Users\brenn\AppData\Local\Temp\devspecs-k01-1-smoke-20260617-154911\repo`
- Isolated home: `C:\Users\brenn\AppData\Local\Temp\devspecs-k01-1-smoke-20260617-154911\home`
- Commands run: `version`, root `--help`, `update`, `tldr brownfield`, `init --yes --tool codex,cursor,claude,windsurf --index foreground`, pre-task `find/recent/map`, `task`, `task status`, `task prompt`, `apply`, `task checkpoint`, post-checkpoint `apply`, post-task `find`, scoped `map Auth`.

## Validation Results
- Root help excluded `list`, `capture`, `eval`, `resume`, and `todos`.
- `ds tldr brownfield` put `ds task "implement <bounded target>"` before `ds map`, `ds recent`, and `ds find`.
- `ds update` returned guidance only and did not mutate the install.
- `ds init` generated:
  - `.agents/skills/ds-task/SKILL.md`
  - `.agents/skills/ds-apply/SKILL.md`
  - `.cursor/commands/ds-task.md`
  - `.cursor/commands/ds-apply.md`
  - `.claude/skills/ds-task/SKILL.md`
  - `.claude/skills/ds-apply/SKILL.md`
  - `.windsurf/workflows/ds-task.md`
  - `.windsurf/workflows/ds-apply.md`
- Pre-task `ds find "active decision token refresh retry"` included `docs/plans/PLAN-AUTH-RETRY.md`, `docs/notes/active-decision.md`, and `src/auth/token.py`.
- `ds recent --json` surfaced the token refresh topic with key paths including the active decision memo and blocked historical plan.
- `ds map --json` and `ds map Auth --json` returned an auth-related architecture boundary with source/docs evidence.
- `ds task "add retry guard for token refresh"` created `launch-smoke` with `A01` and `A02` slices and packed `src/auth/token.py`, `tests/test_token.py`, and `docs/plans/PLAN-AUTH-RETRY.md`.
- `ds apply launch-smoke --json` selected `A01` before checkpoint and `A02` after checkpointing `A01` with `--decision promote`.
- Post-task `ds find "PLAN-AUTH-RETRY token refresh retry"` included the owner plan and generated `devspecs/tasks/launch-smoke` workspace.
- No focused-repo `Workspace root warning` appeared.

## Residual Caveats
- `ds task` still writes normal progress (`Task index updated...`) to stderr. This is acceptable for launch but still worth tightening later for script UX.
- `ds init` still prints the sparse-docs suggestion in the tiny fixture. This is noisy in synthetic repos but less concerning in real launch repos.
- This smoke verified generated Codex/Cursor/Claude/Windsurf files, not live execution inside every external agent shell.

## Changed Files
- `devspecs/tasks/v1-1-release-readiness-tag-gate/task.json`
- `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-repeat-launch-smoke-after-command-surface-and-di-plan.md`
- `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-repeat-launch-smoke-after-command-surface-and-di-result.md`
- `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-release-smoke-transcript.md`

## Tests
- Smoke: isolated K01-1 PowerShell harness, 16 CLI commands, all exit `0`.
- Check: root help hidden-command guard.
- Check: generated agent-file existence guard.
- Check: task/apply progression guard (`A01` before checkpoint, `A02` after `promote`).
- Check: pre-task owner-intent retrieval guard.
- Check: focused repo workspace-warning guard.

## Decision
- Promote.

## Follow-up
- Proceed to K02 public docs/tldr update, then K03 final tag gate.

## References
- `K00-index.md`
- `K01-1-repeat-launch-smoke-after-command-surface-and-di-plan.md`
- `K01-1-release-smoke-transcript.md`

## Checkpoints
- Use `ds task checkpoint v1-1-release-readiness-tag-gate --target K01-1` to append structured evidence.

### Checkpoint
- Created At: 2026-06-17T13:52:01Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260617-135201-validated.md`
- Structured Evidence: `checkpoints/20260617-135201-validated.json`
- Note: Proceed to K02 public docs/tldr, then K03 tag gate.
- What changed: K01-1 updated smoke passed in a fresh isolated repo after command-surface and diagnostic-ranking repairs.
- Evidence for decision: 1 file(s) read; 3 file(s) edited; 1 test command(s)
- What remains: next target K02; next decision promote
- Next iteration: K02 with decision promote
- Files read:
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-release-smoke-transcript.md`
- Files edited:
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-repeat-launch-smoke-after-command-surface-and-di-plan.md`
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-repeat-launch-smoke-after-command-surface-and-di-result.md`
  - `devspecs/tasks/v1-1-release-readiness-tag-gate/K01-1-release-smoke-transcript.md`
- Tests run:
  - `isolated K01-1 PowerShell smoke harness: 16 CLI commands, all exit 0`
