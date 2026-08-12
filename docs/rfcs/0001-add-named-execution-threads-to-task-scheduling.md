# RFC: Add named execution threads to task scheduling

## Status

Reopened

The original repo-task-only proposal was accepted on 2026-08-12, then reopened
before implementation after dogfood exposed two incorrect assumptions: threads
may span repositories, and mutable lifecycle state should not depend on repeated
`task.json` rewrites.

## Summary

Add named execution threads as ordered lanes of existing DevSpecs targets with
optional dependencies between lanes. Preserve repo-first behavior for ordinary
tasks. When a task is explicitly linked to a workspace change through its
existing `workspace_id`, `parent_change`, and `repo_alias` metadata, the
workspace change owns any cross-repository thread graph.

Keep durable lifecycle authority in immutable per-checkpoint JSON records.
Build thread readiness and current target state as a rebuildable SQLite
projection, with direct file reconstruction available when the projection is
missing or stale. A remote service such as Kleio may aggregate the same local
records later, but is never required for mutation or scheduling.

## Motivation

`ds task next` currently walks task slices in manifest order and returns one
global target. It cannot represent two independent lanes, a target that waits
for both, or another split after the join.

The first proposal placed `threads` directly in one task's `task.json` and
serialized concurrent task mutations around that file. That would work for one
repository, but it gives the wrong owner to lanes that include targets from
multiple workspace child repositories. It also deepens an existing storage
problem: `task.json` currently mixes comparatively static task definition with
frequently updated lifecycle state and checkpoint pointers.

## Goals

- Represent serial lanes, parallel splits, joins, and later splits without an
  arbitrary global priority.
- Default to a repo-local task owner when no workspace change link exists.
- Let an existing workspace change own a graph that addresses targets across
  linked child repositories.
- Keep checkpoint gates as the source of completion truth.
- Make SQLite status and scheduling fast while remaining fully rebuildable from
  local artifacts.
- Make concurrent checkpoint creation append-only and loss-safe.
- Preserve legacy task behavior when no thread graph exists.

## Non-goals

- Requiring users to initialize a workspace for ordinary repository work.
- Launching, assigning, claiming, monitoring, or terminating agent processes.
- Making the global SQLite index or a remote service authoritative.
- Adding thread-specific plans, results, checkpoints, stages, or decisions.
- Replacing follow-up slices or the once-per-track durable record closeout.

## Proposed authority model

### Repo-first scope resolution

1. A task without `parent_change` uses a repo-local thread definition owned by
   that task.
2. A task linked to a workspace change resolves thread ownership through the
   existing workspace ID, parent change ID, and repo alias.
3. A workspace merely containing a repository does not take ownership of plain
   repo tasks. The explicit task-to-change link is the boundary.
4. Repo and workspace definitions may not both own the same target.
5. No workspace is created implicitly to support threads.

The exact companion-file layout and cross-repo target-address syntax are
reopened F01 decisions. Thread definitions remain small, declarative local
artifacts rather than mutable scheduler state.

### Durable lifecycle events

Each checkpoint JSON file is one immutable lifecycle event containing its
checkpoint ID, task and target identity, stage, decision, evidence, and
workspace link when present. Writers create uniquely named files with exclusive
publication; they do not append to one shared event file.

A shared JSONL file is not the authoritative format. Concurrent appends can
interleave or truncate, one corrupt line complicates recovery, and Git merges
become a shared hotspot. JSONL may be offered later as a deterministic export
or ingestion stream assembled from the individual event files.

Legacy lifecycle fields in `task.json` remain readable during migration. The
reopened design must define precedence, dual-write duration, and diagnostics
for disagreement before implementation proceeds.

### SQLite projection

The local DevSpecs database projects task definitions, checkpoint events,
workspace links, thread definitions, and derived readiness. It supports fast
status and discovery across repositories, but every projected row can be
reconstructed from local artifacts. `ds prune`, index rebuilds, a different
`DEVSPECS_HOME`, or database loss must not erase scheduling authority.

Commands may incrementally ingest newly written events. On a cold or stale
index, they must reconstruct enough state from the owning repo task or workspace
change to return the same result before updating the projection.

### Thread semantics

A thread contains ordered target references and may depend on other threads
with `after`. Keys and target membership are unique within one owner,
dependencies must exist, self-dependencies and cycles are invalid, and series
closeout targets cannot be assigned. Follow-ups inherit their parent target's
thread and run before the lane advances.

Thread state is derived as `ready`, `active`, `waiting`, `blocked`, or
`completed`. Improve, rework, block, and rollback remain unresolved and do not
satisfy a join. Definition order controls deterministic presentation only; it
is never a global priority.

## Proposed CLI

```text
ds thread [owner]
ds thread set <owner> <key> <target>... [--name <label>] [--after <key>]...
ds thread remove <owner> <key>
ds apply [task-id] --thread <key>
```

`owner` resolves to a repo task by default or an explicitly linked workspace
change. F01 must settle how cross-repo targets are addressed and how commands
behave when invoked from a child repository. No duplicate `ds workspace thread`
surface is proposed.

Status reports all ready lanes and explains why other lanes wait. Apply without
a selector continues only when a legacy linear task or exactly one runnable
lane makes the choice unambiguous. Explicit targets may not bypass dependencies.

## Compatibility

- Legacy manifests require no eager migration.
- A task with no explicit thread definition keeps linear scheduling.
- `ds task next` remains a compatibility path for legacy or unambiguous work,
  but never chooses arbitrarily among multiple ready lanes.
- Missing SQLite state triggers local reconstruction rather than weaker output.
- Existing workspace slice links are reused; plain repo tasks stay repo-local.
- Compose documents and prune/index maintenance retain their ownership.

## Alternatives

- Global SQLite as authority: transactionally convenient, but database rebuild,
  prune, home isolation, or corruption would erase durable orchestration.
- One task-local mutable JSON manifest: inspectable, but a concurrency hotspot
  and unable to naturally own cross-repo lanes.
- One shared JSONL event log: stream-friendly, but still a shared append and Git
  merge hotspot.
- Task-local SQLite authority: transactional and local, but opaque to Git and
  awkward for workspace graphs spanning several repositories.
- Mandatory workspace ownership: one graph model, but needless ceremony for
  ordinary repo work and inconsistent with current repo-first behavior.
- Remote Kleio authority: useful aggregation, but violates offline local-first
  operation. Optional replication remains compatible with this proposal.

## Risks and rollout

The largest risk is dual authority during migration. F01 must define one
deterministic precedence rule and a diagnostic for disagreement. The second
risk is making cold reconstruction materially slower or weaker than the SQLite
projection; equivalent first-result semantics are a release gate.

The command remains experimental for v1.4. Rollout must prove standalone repo
tasks and linked cross-repo changes independently before hiding `ds task next`.

## Reopened decisions

- Repo-local companion-file location and schema.
- Workspace-change companion-file location and schema.
- Cross-repo target-address syntax and child-repo command inference.
- Migration from mutable `task.json` lifecycle fields to checkpoint authority.
- SQLite projection schema, freshness rules, and cold reconstruction budget.
- Guarded graph mutation once checkpoint history exists.

<!-- devspecs: task=threaded-task-orchestration target=F01 -->
