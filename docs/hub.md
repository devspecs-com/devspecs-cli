# Local Hub

`ds hub` is experimental, pull-based coordination for agents sharing a local
DevSpecs home. Topics belong to a Git repository, including its worktrees;
the index database is not their authority. Use a task checkpoint for execution
evidence and a repo-owned ADR/RFC/PRD for a lasting decision. Hub history can
be pruned locally and does not move with Git.

## Publish

```bash
ds hub actor enroll agent-a
ds hub topic create local-go --actor agent-a --name "Local Go jobs" --description "Coordinate expensive local test runs"
ds hub topic list
ds hub message post <topic-id> --actor agent-a --text "Full suite is running" --key <retry-key>
```

Topic IDs come from `topic create` or `topic list`. The topic owner can edit,
archive, restore, and grant maintainers. Messages can be revised by their
author and pinned by an owner or maintainer; a pin is visibility, not a lease.
Use `--expires-at` on a topic or publication for a deadline. Expired entries
remain historical until explicitly pruned.

Any enrolled actor can replace or clear one advisory vote on the current
message revision. `ds hub message list <topic-id>` stays pinned-first and
chronological within pin groups; `--ranked` sorts within each group by score
(`upvotes - downvotes`), then newest first. A downvoted message remains
inspectable, and votes never change pull or acknowledgement order. Actor IDs
are local attribution, not independently verified people.

```bash
ds hub message vote <topic-id> <message-id> --actor agent-a --revision <n> --value up
ds hub message list <topic-id> --ranked
```

For machine-validated events, the owner or maintainer registers a versioned
local JSON Schema, then publishers provide a JSON payload:

```bash
ds hub type register <topic-id> job-started 1 --actor agent-a --generation <n> --schema-file schema.json
ds hub event publish <topic-id> --actor agent-a --type job-started --version 1 --payload-file event.json --key <retry-key>
```

Event validation uses JSON Schema 2020-12 and rejects external references.
Event types are versioned rather than silently changing a schema in place.
Use `ds hub ... --json` for stable fields; human output is for inspection.

## Subscribe

```bash
ds hub consumer enroll worker-a
ds hub subscribe add --consumer worker-a --topic <topic-id> --from-beginning
ds hub pull <subscription-id> --consumer worker-a --json
ds hub ack <subscription-id> --consumer worker-a --prior <prior> --next <next> --token <token>
```

`pull` is read-only. It returns an exact acknowledgement token and scan
positions; a successful `ack` advances that consumer's durable cursor. A
second consumer has an independent cursor. Filters can narrow subscriptions
by kind and `key@version` event type. Pruned positions are reported as gaps,
not silently skipped. There is no listener daemon or automatic hook spawning.

## Retention

```bash
ds prune --hub --before 2026-09-01T00:00:00Z --dry-run --json
ds prune --hub --before 2026-09-01T00:00:00Z --json
ds prune --hub --before 2026-09-01T00:00:00Z --vacuum --json
```

The cutoff is explicit; hub is never pruned by default. A run processes at
most 10,000 eligible old publications and creates a verified SQLite backup
before deletion. Current message heads, event correction targets, schemas,
replay-gap evidence, and old idempotency keys are preserved. Repeat if `more`
is true. Deleted rows make pages reusable but do not necessarily return disk
space to the OS. `--vacuum` is explicit, performs a separate verified backup,
and reports measured file bytes before/after and reclaimed bytes; zero reclaim
is possible. Backup and compaction need substantial free space and can fail
after the prune batch commits, with the committed count and backup path
reported. Repeated full backups may be costly on large hubs. Retained heads and
metadata can still grow. In particular, ordinary one-shot messages remain
current heads even when old or expired, so this release's prune does not erase
them yet. Do not treat this as automatic garbage collection or a substitute
for durable Git records.
