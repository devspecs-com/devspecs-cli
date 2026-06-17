# K01 Release Smoke Transcript

Date: 2026-06-17
Binary: `ds dev (commit: 9ec6124, built: 2026-06-16T17:32:30Z)`

## Fixture
Temporary repo:

```text
C:\Users\brenn\AppData\Local\Temp\devspecs-k01-smoke-20260617-141214
```

Fixture contents:
- `src/auth/token.py`
- `tests/test_token.py`
- `docs/plans/PLAN-AUTH-RETRY.md`

The smoke used a repo-local `DEVSPECS_HOME` inside the temp repo so it did not depend on the existing DevSpecs index.

## Commands
All 10 core CLI smoke commands exited `0`.

```powershell
ds init --yes --tool all --index foreground
ds map
ds map --json
ds recent
ds find "token refresh retry" --json
ds task --id launch-smoke --profile code-change --slice "Trace token refresh behavior" --slice "Add retry guard" --json "add retry for token refresh"
ds task prompt launch-smoke --json
ds apply launch-smoke --json
ds task checkpoint launch-smoke --target A01 --stage validated --decision promote --file-read src/auth/token.py --file-read tests/test_token.py --test-run "pytest tests/test_token.py" --index=false --json
ds apply launch-smoke --json
```

## Representative Output

### init
Generated all selected agent tooling files:

```text
.agents/skills/ds-task/SKILL.md [created, $ds-task]
.agents/skills/ds-apply/SKILL.md [created, $ds-apply]
.cursor/commands/ds-task.md [created, /ds-task]
.cursor/commands/ds-apply.md [created, /ds-apply]
.claude/skills/ds-task/SKILL.md [created, /ds-task]
.claude/skills/ds-apply.md [created, /ds-apply]
.windsurf/workflows/ds-task.md [created, /ds-task]
.windsurf/workflows/ds-apply.md [created, /ds-apply]
Next:
  /ds-task "goal"
```

Observation: `init` still printed `Suggestion: docs/ looks sparse for specs/plans` even though `docs/plans/PLAN-AUTH-RETRY.md` existed. This is not a blocker, but the copy may be too blunt for sparse-but-valid repos.

### map
`ds map` produced a low-confidence but useful boundary:

```text
Repo map: devspecs-k01-smoke-20260617-141214
Confidence: low
Evidence: 1 markdown, 2 source, Git history

Candidate subsystems

1. Subsystem: Workspace Identity, Access & Billing
   Boundary: docs/plans/**, src/auth/**
   Key files:
   - src/auth/token.py
   - docs/plans/PLAN-AUTH-RETRY.md
   Try: ds find "auth token retry"
   Try: ds task "modify workspace identity, access & billing auth"
```

Verdict: acceptable for v1.1 as architecture/boundary orientation. It should not be marketed as perfect hierarchy yet.

### recent
`ds recent` found the current git topic:

```text
Recently active topics
Repo: devspecs-k01-smoke-20260617-141214

1. Token Refresh
   Evidence: 1 commit, 3 files, source + docs
   Key files:
   - src/auth/token.py
   - tests/test_token.py
   - docs/plans/PLAN-AUTH-RETRY.md
   Try: ds find "token refresh"
```

Verdict: launch-ready diagnostic/evidence layer.

### find
`ds find "token refresh retry" --json` returned a pack with:
- `implementation_surface`: `src/auth/token.py`
- `open_work`: `docs/plans/PLAN-AUTH-RETRY.md`
- git receipt: `add token refresh plan`

Verdict: launch-ready diagnostic/evidence layer. The pack was small and on target.

### task
`ds task ... --json` created `launch-smoke` with two slices:
- `A01` Trace token refresh behavior
- `A02` Add retry guard

Packed context included:
- primary: `src/auth/token.py`
- tests: `tests/test_token.py#L4`, `tests/test_token.py`
- docs: `docs/plans/PLAN-AUTH-RETRY.md`

Observation: when stderr and stdout were captured together in PowerShell, the normal progress line `Task index updated (1 new, 0 updated)` appeared as a `NativeCommandError` wrapper even though exit code was `0` and JSON stdout was otherwise valid. This is a launch papercut for scripted Windows users.

### prompt / apply
`ds task prompt launch-smoke --json` and `ds apply launch-smoke --json` both emitted a bounded one-slice prompt for `A01`:

```text
You are working on DevSpecs task launch-smoke target A01 only.
must_not_implement:
  - A02
At the end, recommend exactly one decision: promote, improve, rework, rollback, or block.
```

After checkpointing `A01` with `--decision promote`, `ds apply launch-smoke --json` correctly selected `A02`.

Verdict: launch-ready prompt-only apply loop.

## Agent Tooling Smoke

Generated files were present:

```text
.agents/skills/ds-task/SKILL.md
.agents/skills/ds-apply/SKILL.md
.cursor/commands/ds-task.md
.cursor/commands/ds-apply.md
.claude/skills/ds-task/SKILL.md
.claude/skills/ds-apply.md
.windsurf/workflows/ds-task.md
.windsurf/workflows/ds-apply.md
```

External CLI probes:
- `codex --help` failed with `Access is denied` for the Windows App package executable, so Codex slash/skill execution could not be smoke-tested from this shell.
- `claude --help` succeeded.
- A constrained Claude non-interactive stdin probe with `/ds-task "add retry for token refresh"` succeeded at the CLI level and produced a plausible first response, but tools were disabled and it did not execute the generated DevSpecs command. Treat this as partial smoke only, not proof of full Claude interactive slash command behavior.

## Launch Findings

Promote-worthy:
- `task`, `prompt`, `apply`, `find`, `recent`, `map`, and `init` all worked in a fresh repo.
- `ds apply` respected decision-gated progression from `A01` to `A02`.
- Generated agent files were complete across Codex, Cursor, Claude, and Windsurf paths.

Needs improvement before final tag:
- Root help still exposes top-level `ds list`, which conflicts with the task-first launch story. Planned as M track: `task-list-inventory-ux`.
- Windows/PowerShell combined-stream capture can make normal `ds task` progress output look like an error.
- `init` sparse-docs suggestion may be noisy for tiny but valid repos.
- Codex CLI slash smoke is blocked by local Windows app permissions; Claude CLI smoke is partial without running an interactive model/tool session.

## Decision
K01 should be `improve`, not `promote`.

Next release route remains:

```text
K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03
```
