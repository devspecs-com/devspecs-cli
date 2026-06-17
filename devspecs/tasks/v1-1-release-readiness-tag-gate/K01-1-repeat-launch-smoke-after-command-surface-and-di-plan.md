# Task v1-1-release-readiness-tag-gate K01-1 Plan

## Goal
Repeat the launch smoke after the command-surface, stale-warning, map/recent, and find-ranking repairs.

## Context
K01 validated the core launch flow but held at `improve` until the planned repair route completed:

```text
K01 -> B03 -> H02 -> H03 -> F03 -> K01-1 -> K02 -> K03
```

K01-1 is the release-readiness retest. It should use the current local binary in a fresh repo with an isolated `DEVSPECS_HOME`, not this repository's existing database.

## Smoke Contract
- Root help presents the v1.1 launch surface: `task`, `apply`, `init`, `tldr`, `find`, `recent`, `map`, `update`, plus supporting `show/context/scan/config/version`.
- Removed or hidden commands stay out of root help: `list`, `capture`, `eval`, `resume`, `todos`.
- `ds tldr brownfield` puts `ds task` before diagnostic commands.
- `ds update` returns guidance without mutating the install.
- `ds init --yes --tool codex,cursor,claude,windsurf --index foreground` writes the expected agent command/skill/workflow files and prints one next step.
- Pre-task `ds find`, `ds recent`, and `ds map` recover owner intent from a brownfield fixture with an active decision memo, a next plan, a blocked historical plan, source, and tests.
- `ds task` creates a bounded two-slice workspace with packed source/test/docs context.
- `ds task prompt` and `ds apply` emit the same one-slice boundary for `A01`.
- A `promote` checkpoint on `A01` moves `ds apply` to `A02`.
- Post-task `ds find` includes the owner plan and generated task workspace without losing source/test context.
- Focused repo commands do not emit the workspace-root warning.

## Fixture
Use a temporary repo containing:

- `docs/plans/PLAN-AUTH-RETRY.md` with `Status: next`
- `docs/notes/active-decision.md` with `Status: active`
- `docs/plans/D4.2-blocked-oauth-migration.md` with `Status: blocked`
- `src/auth/token.py`
- `tests/test_token.py`

## Success Criteria
- [ ] Every smoke command exits `0`.
- [ ] Help excludes hidden launch-noise commands.
- [ ] Agent tooling files are generated for Codex, Cursor, Claude, and Windsurf.
- [ ] Pre-task find includes both the owner plan and active decision memo.
- [ ] Apply selects `A01` before checkpoint and `A02` after checkpoint.
- [ ] No focused-repo workspace-root warning appears.
- [ ] Result records residual caveats instead of treating smoke coverage as proof of perfect UX.

## Decision Gates
- Promote: the v1.1 task-first launch flow passes and remaining caveats are non-blocking for docs/tag gate.
- Improve: any command fails, help regresses, task/apply gating regresses, owner intent is not recoverable, or focused repo warnings reappear.
- Rework: the smoke contract itself is mismatched to the launch story.
- Rollback: the current binary regresses versus K01 on core task/apply/find behavior.
- Block: external tooling or release infrastructure prevents a meaningful smoke.
