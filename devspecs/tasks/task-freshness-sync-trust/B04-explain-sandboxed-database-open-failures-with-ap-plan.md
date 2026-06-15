# Task task-freshness-sync-trust B04 Plan

## Goal
Explain sandboxed database-open failures with approval and DEVSPECS_HOME guidance

## Description
Improve the error shown when a sandboxed agent cannot open the global DevSpecs SQLite database. The CLI should explain the likely permission boundary and give two useful exits: rerun with filesystem approval, or set `DEVSPECS_HOME` to a writable gitignored directory.

## Resources
- `B00-index.md`
- `B04-explain-sandboxed-database-open-failures-with-ap-result.md`
- `task.json`

## Starting Context
### Evidence to Review
- `internal/commands/list.go` owns the shared `openDB()` helper used by most commands.
- `internal/commands/scan.go` opens the DB directly after resolving `config.DBPath()`.
- `internal/store/store.go` already wraps SQLite busy/locked errors with a friendly concurrent-writer message.

### Test or Evaluation Signals
- Unit tests should cover access-looking DB-open failures, busy errors, and unrelated errors.
- Existing busy-error behavior must remain intact.

## Expected Change Surface
- `internal/commands/list.go`
- `internal/commands/scan.go`
- focused command tests

## Out-of-Scope Areas
- Moving the default DB into the repo.
- Silent fallback to a repo-local DB.
- Full sandbox detection for every agent runtime.
- Changing SQLite busy/lock handling.

## Risks
- Overstating certainty: not every DB-open failure is caused by sandboxing.
- Making the message too long for agents to act on.
- Accidentally replacing the friendlier SQLite busy message.

## Success Criteria
- [ ] DB-open access/permission failures mention filesystem sandbox, approval, and `DEVSPECS_HOME`.
- [ ] Busy/locked SQLite errors still mention another `ds` command writing.
- [ ] `ds scan` gets the same friendly error path as commands that use `openDB()`.
- [ ] Focused tests cover the message branches.

## Tasks
- [ ] Centralize DB-open error wrapping behind `openDBAtPath`.
- [ ] Add sandbox/access failure wording without suppressing the original error.
- [ ] Route explicit `ds scan` through the same helper.
- [ ] Add focused tests for access, busy, and unrelated errors.
- [ ] Update `B04-explain-sandboxed-database-open-failures-with-ap-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the error becomes actionable without changing DB location semantics.
- Improve: wording is useful but needs shorter copy or docs follow-up.
- Rework: implementation implies unsafe repo-local fallback behavior.
- Rollback: message becomes misleading for common non-sandbox DB failures.
