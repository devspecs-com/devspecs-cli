# ADR-0002: Keep doctor diagnostics read-only

## Status

Accepted

## Context

DevSpecs support failures often involve the active executable, PATH precedence,
local SQLite compatibility, writer contention, or repository identity. The
index may be missing, locked, newer than the running CLI, or unreadable at the
moment diagnostics are most valuable. A diagnostic that opens the normal store
can create state or run migrations, while automatic repair can destroy the
evidence needed to choose the correct recovery.

Diagnostics and recovery therefore need separate trust boundaries. The next
recovery track may add explicit backup or rollback operations, but those
operations must not make a support report dependent on mutation succeeding.

## Decision

`ds doctor` is offline and read-only. It collects one typed report and renders
all independent evidence before returning an error status. Human and JSON
output share the same findings and bounded remediation model.

Doctor uses a separate read-only SQLite inspection path. It does not call the
normal migrating store open path, create DevSpecs files or directories,
acquire a missing writer lock, scan, prune, update, invoke a package manager,
repair the index, or make network requests. Recommended commands label whether
they are read-only, mutating, or destructive and run only after a user or agent
invokes them separately.

Local paths and repository identifiers remain useful in local output but are
structurally transformed by `--redact`. Raw Git remotes, environment values,
repository contents, and queries are excluded. Doctor does not emit telemetry
because telemetry identity initialization may itself create local state.

## Consequences

- Positive: A report remains trustworthy when the index is absent,
  incompatible, contended, or damaged, and failed checks do not hide unrelated
  runtime evidence.
- Positive: Support output can be shared with structural redaction without
  turning diagnostics into another source of index mutation.
- Negative: Doctor cannot offer a one-command repair, and read-only inspection
  requires APIs separate from normal store initialization.
- Follow-up: Backup, rollback, downgrade recovery, and other mutations belong
  to explicit commands in the `index-downgrade-recovery` track. Their tests
  must preserve this diagnostic boundary.

<!-- devspecs: task=cli-doctor-diagnostics target=H00 -->
