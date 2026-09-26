# ADR-0002: Keep hub coordination separate from the source index

## Status

Accepted for the experimental local hub

## Context

The DevSpecs source index is derived from repository files and can be rebuilt.
Task checkpoints and composed decisions live in Git. Agent coordination is
different: a short message, validated event, or subscription acknowledgement
may need to survive a source-index rebuild, but should not create a repository
file for every publication. Ephemeral worktrees of one Git repository should
see the same local topics.

## Decision

`ds hub` uses a separate SQLite authority in the selected DevSpecs home.
Topics resolve repo-first across worktrees through the Git common directory
identity. Messages and events share a scoped publication sequence, and each
consumer has an independent, explicit pull/ack cursor. There is no implicit
listener process, cross-machine replication, task promotion, or blocking
resource lock. An optional workspace does not change ordinary repo ownership.

Hub publications are local coordination, not durable architectural records.
The operator may explicitly prune eligible old publications by cutoff after a
verified backup; default source-index pruning never touches the hub. Current
heads, schema definitions, replay-gap evidence, and idempotency tombstones
remain until a separately reviewed retention policy can safely remove them.
Decisions that must survive local home loss belong in Git-owned task receipts
or composed documents.

## Consequences

- Positive: Source-index rebuilds and temporary worktree removal do not erase
  local coordination or subscriber progress.
- Positive: Agents can exchange validated events without installing a daemon
  or modifying an orchestration framework.
- Negative: A different machine or DevSpecs home cannot see these topics; the
  hub is not a distributed message bus.
- Negative: Local authority needs its own backup, migration, disk-retention,
  and recovery contract. Row pruning alone may not shrink the SQLite file.
- Follow-up: Prove compaction and growth bounds before describing hub pruning
  as complete disk garbage collection.
