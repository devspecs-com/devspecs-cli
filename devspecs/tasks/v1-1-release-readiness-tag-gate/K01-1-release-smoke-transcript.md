# K01-1 Release Smoke Transcript

## Setup
- Binary: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli\.devspecs\bin\ds.exe`
- Version: `ds dev (commit: 87d8731, built: 2026-06-17T13:40:54Z)`
- Fixture repo: `C:\Users\brenn\AppData\Local\Temp\devspecs-k01-1-smoke-20260617-154911\repo`
- Isolated DevSpecs home: `C:\Users\brenn\AppData\Local\Temp\devspecs-k01-1-smoke-20260617-154911\home`
- Fixture commit: `add token refresh retry context`

## Fixture Files
```text
docs/plans/PLAN-AUTH-RETRY.md
docs/notes/active-decision.md
docs/plans/D4.2-blocked-oauth-migration.md
src/auth/token.py
tests/test_token.py
```

## Commands
```powershell
ds version
ds --help
ds update
ds tldr brownfield
ds init --yes --tool codex,cursor,claude,windsurf --index foreground
ds find "active decision token refresh retry" --json
ds recent --json
ds map --json
ds task "add retry guard for token refresh" --id launch-smoke --series A --slice "Trace token refresh behavior" --slice "Add retry guard" --json
ds task status launch-smoke --json
ds task prompt launch-smoke --json
ds apply launch-smoke --json
ds task checkpoint launch-smoke --target A01 --stage validated --decision promote --description "Smoke validated bounded first slice." --test-run "go test ./..." --json
ds apply launch-smoke --json
ds find "PLAN-AUTH-RETRY token refresh retry" --json
ds map Auth --json
```

## Assertions
- Step count: 16
- All command exits: `0`
- Hidden commands absent from root help: `list`, `capture`, `eval`, `resume`, `todos`
- `ds tldr brownfield` task-first ordering: pass
- Tooling files generated: Codex, Cursor, Claude, Windsurf task/apply files
- Pre-task find includes owner plan: pass
- Pre-task find includes active decision memo: pass
- Post-task find includes owner plan: pass
- Post-task find includes generated task workspace: pass
- `ds apply` before checkpoint target: `A01`
- `ds apply` after checkpoint target: `A02`
- Focused repo workspace warning seen: `false`
- Non-empty stderr steps: `08-task-create` only, with normal progress text

## Selected Output
```text
Initialized DevSpecs.
...
Generated files:
  - .claude/skills/ds-task/SKILL.md [created, /ds-task]
  - .claude/skills/ds-apply/SKILL.md [created, /ds-apply]
  - .agents/skills/ds-task/SKILL.md [created, $ds-task]
  - .agents/skills/ds-apply/SKILL.md [created, $ds-apply]
  - .cursor/commands/ds-task.md [created, /ds-task]
  - .cursor/commands/ds-apply.md [created, /ds-apply]
  - .windsurf/workflows/ds-task.md [created, /ds-task]
  - .windsurf/workflows/ds-apply.md [created, /ds-apply]
Next:
  /ds-task "goal"
```

```text
pre-task find:
- docs/plans/PLAN-AUTH-RETRY.md
- docs/notes/active-decision.md
- src/auth/token.py
```

```text
apply before checkpoint: A01
apply after checkpoint: A02
```

## Verdict
K01-1 passes the updated launch smoke. Promote to K02.
