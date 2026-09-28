# Local Hub

`ds hub` is experimental, pull-based coordination for agents sharing a local
DevSpecs home. Topics belong to a Git repository, including its worktrees, by
default. Prefix a topic address with `global:` to opt into the one home-global
scope shared by unrelated repositories using that same home. The index
database is not their authority. Use a task checkpoint for execution
evidence and a repo-owned ADR/RFC/PRD for a lasting decision. Hub history can
be pruned locally and does not move with Git.

## Publish

```bash
ds hub actor enroll agent-a
ds hub topic create local-go --actor agent-a --name "Local Go jobs" --description "Coordinate expensive local test runs"
ds hub topic list
ds hub message post <topic-id> --actor agent-a --text "Full suite is running" --key <retry-key>
```

For cross-repository machine coordination, use the prefix explicitly at each
scope-bearing step:

```bash
ds hub topic create global:local-go --actor agent-a --name "Local Go jobs" --description "Coordinate expensive local test runs"
ds hub topic list global:
ds hub message post global:<topic-id> --actor agent-a --text "Full Go suite running" --key <retry-key>
ds hub subscribe add --consumer worker-a --topic global:<topic-id> --from-beginning
ds hub pull global:<subscription-id> --consumer worker-a --json
ds hub ack global:<subscription-id> --consumer worker-a --prior <prior> --next <next> --token <token>
```

Unprefixed addresses remain repo-scoped. A global subscription cannot mix
repo and global topics. `global:` is an address prefix, not part of the stored
topic key, and it does not create a cross-machine service. Different
`DEVSPECS_HOME` values have separate global scopes. Messages about heavy work
are advisory; they do not reserve resources or block concurrent jobs. Use an
external scheduler until a real lease/fencing mechanism is implemented.

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
before deletion. Old unpinned messages, including one-shot messages, and
whole old event correction chains can be removed. Pinned messages, newer
chains, schemas, replay-gap evidence, and old idempotency keys are preserved.
Repeat if `more` is true. Deleted rows make pages reusable but do not
necessarily return disk space to the OS. `--vacuum` is explicit, performs a
separate verified backup, and reports measured file bytes before/after and
reclaimed bytes; zero reclaim
is possible. Backup and compaction need substantial free space and can fail
after the prune batch commits, with the committed count and backup path
reported. Repeated full backups may be costly on large hubs. A single message
or correction group above 10,000 publications cannot be pruned yet. Pinned
heads and metadata can still grow. Do not treat this as automatic garbage
collection or a substitute for durable Git records.

An interrupted prune or vacuum can leave a pending backup intent. A later
explicit prune or vacuum reconciles an owned backup only if no audit record
retains it; opening the hub does not run garbage collection. Safe backup
ownership requires hard-link support in the hub home. Without it, backup
creation fails closed.
