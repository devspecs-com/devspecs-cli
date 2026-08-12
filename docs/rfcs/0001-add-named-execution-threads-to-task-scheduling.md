# RFC: Add named execution threads to task scheduling

## Status

Accepted

The original repo-task-only proposal was accepted, then reopened before
implementation when dogfood exposed two incorrect assumptions: threads may span
repositories, and mutable lifecycle state should not depend on repeated
`task.json` rewrites. This revision records the corrected repo-first,
workspace-capable contract.

## Summary

Add named execution threads as ordered lanes of existing DevSpecs targets with
optional dependencies between lanes. A standalone task owns a repo-local
`threads.yaml`. A task explicitly linked to a workspace change participates in
that change's `<change-id>.threads.yaml`, which may address targets in several
linked repositories. Merely living under a workspace root does not transfer
ownership.

Keep durable lifecycle authority in immutable per-checkpoint JSON events. Use
the local SQLite database as a rebuildable projection for fast status and
scheduling. Cold reconstruction from local artifacts must produce the same
scheduler result. A remote service such as Kleio may aggregate those artifacts
later, but is never required for local mutation or scheduling.

## Motivation

`ds task next` walks slices in manifest order and returns one global target. It
cannot represent parallel lanes, a join that waits for both, or another split
after that join.

Putting the graph and lifecycle state in one task's `task.json` would deepen an
existing storage problem. That file mixes task definition with frequently
updated stage, decision, and checkpoint pointers, and one repository cannot
naturally own a graph spanning multiple child repos. The corrected design
separates graph definitions, immutable lifecycle events, and rebuildable query
state.

## Goals

- Represent serial lanes, parallel splits, joins, and later splits without an
  arbitrary global priority.
- Default to repo-local ownership when no workspace change link exists.
- Let an existing workspace change own a graph across linked repositories.
- Keep checkpoint gates as the source of completion truth.
- Make SQLite status fast without creating a warm-only quality tier.
- Make concurrent checkpoint creation append-only and loss-safe.
- Preserve legacy linear tasks without eager migration.

## Non-goals

- Requiring workspace initialization for ordinary repository work.
- Launching, assigning, claiming, monitoring, or terminating agents.
- Making SQLite, JSONL, or a remote service authoritative.
- Adding thread-specific plans, results, stages, or decisions.
- Replacing follow-up slices or the once-per-track durable closeout.

## Durable thread definitions

Thread definitions use YAML because they are small, human-reviewed graphs. They
are separate from mutable task state:

- Repo task: `<task-workspace>/threads.yaml`
- Workspace change: `<artifact-dir>/changes/<change-id>.threads.yaml`

The workspace filename uses the immutable change ID rather than the title slug,
so renaming the Markdown change document does not move graph authority.

### Repo example

```yaml
schema_version: 1
revision: 3
owner:
  kind: task
  task_id: checkout-redesign
threads:
  - key: agent-a
    name: Agent A
    targets:
      - target: A01
      - target: A02
  - key: human-a
    name: Human A
    targets:
      - target: A03
  - key: both-a
    targets:
      - target: A04
    after: [agent-a, human-a]
```

### Workspace example

```yaml
schema_version: 1
revision: 2
owner:
  kind: workspace_change
  workspace_id: commerce
  change_id: COM-C014
threads:
  - key: agent-a
    targets:
      - repo: api
        task: COM-C014-api
        target: A01
      - repo: web
        task: COM-C014-web
        target: B01
  - key: both-a
    targets:
      - repo: api
        task: COM-C014-api
        target: A03
    after: [agent-a, human-a]
```

Workspace definitions persist full repo/task/target objects. Repo aliases must
exist in `workspace.yaml`; unaliased `--repo-path` slices cannot join a
cross-repo graph until the repository has an alias. Every referenced task must
link back to the same workspace and parent change. The workspace change's Repo
Slices record and child task metadata are cross-checked; disagreement blocks
scheduling instead of guessing.

Keys and target membership are unique within one owner. Dependencies must
exist, self-edges and cycles are invalid, and closeout targets cannot be
assigned. Follow-ups inherit their parent's thread and run before the lane
advances; they are not copied into the definition.

Definition edits are atomic and increment `revision`. Targets without
checkpoint history may be moved or reordered. Once a target has history, its
thread membership and relative order are pinned. Once any member of a thread
has history, that thread's key and dependencies are pinned. Display names may
change, and new unstarted targets or downstream threads may be appended. A
guarded remove is allowed only when the removed thread has no checkpointed
members and no started dependent thread.

## Owner and target resolution

Bare owner IDs preserve repo-first behavior:

1. An exact task ID resolves to that repo task.
2. If that task has an explicit workspace parent-change link, ownership
   escalates to the linked workspace change.
3. An exact workspace change ID resolves to that change.
4. If a bare ID is ambiguous, the command requires `task:<id>` or
   `change:<id>` and never picks one.
5. With no owner argument, inference is allowed only from one unambiguous active
   task in the current repo. A linked task then resolves its parent change.

A plain repo task remains repo-owned even when its directory happens to be
inside a DevSpecs workspace. No workspace or change is created implicitly.

Repo graphs use existing target selectors such as `A01`. Workspace graph CLI
input uses `<repo-alias>:<target>`, for example `api:A01`. It expands through
the change's durable slice links and is accepted only when one linked task
matches. The explicit `<repo-alias>:<task-id>:<target>` form resolves future or
ambiguous multi-task cases. Durable YAML always stores all three fields.

## Durable lifecycle events

Each checkpoint JSON file is one immutable lifecycle event. Schema v3 adds:

```json
{
  "schema_version": 3,
  "event_kind": "task_checkpoint",
  "checkpoint_id": "cp_...",
  "task_id": "checkout-redesign",
  "target": "A01",
  "supersedes_checkpoint_ids": ["cp_previous"],
  "stage": "validated",
  "decision": "promote",
  "created_at": "2026-08-12T12:00:00Z"
}
```

Normal writes name the current target head. A first event has no predecessor.
Events are published under unique names with exclusive create and atomic final
rename before compatibility projections are updated. A crash after event
publication can leave `task.json` or result Markdown stale, but cannot lose the
authoritative transition.

The current state is the one unsuperseded event head. Multiple heads represent
a real concurrent conflict; timestamps never silently choose a winner. The
target becomes conflicted and its thread becomes blocked. A resolving
checkpoint must name every current head with repeatable `--supersedes`; its
stage and decision become the new state. Normal checkpoint commands infer the
single predecessor automatically.

A shared JSONL file is not authoritative. Concurrent append and Git merge
behavior make it another write hotspot. JSONL may later be generated in stable
event order as an export or replication stream.

### Legacy migration

- If a target has no checkpoint JSON, its existing `task.json` lifecycle fields
  remain the legacy fallback.
- Existing schema v1/v2 checkpoints are treated as an implicit linear history,
  ordered by `created_at` and then `checkpoint_id` for ties.
- When any checkpoint history exists, its head wins over lifecycle fields in
  `task.json`; disagreement is reported as a stale compatibility projection.
- The first v3 event supersedes the computed v1/v2 head.
- During v1.4, successful writes continue updating `task.json`, result Markdown,
  and latest-checkpoint pointers for compatibility, but those are projections.
- Hidden lifecycle aliases such as `start`, `finish`, and `decide` must emit a
  checkpoint event rather than mutate only `task.json`.
- Reads never rewrite legacy artifacts merely because they were inspected.

## SQLite projection

SQLite projects, but does not own:

- owners and durable definition path/revision/digest;
- ordered thread definitions, dependencies, and canonical target addresses;
- checkpoint events, causal predecessors, and event digests;
- repo/workspace links needed to resolve cross-repo targets.

Readiness remains a pure scheduler result computed from projected definitions
and event heads; it is not another persisted lifecycle field.

Projection freshness uses durable source inventories. Definitions store path,
size, modification time, and SHA-256. Checkpoint directories are inventoried by
relative JSON path, size, and modification time; only new or changed files are
parsed and hashed. Missing files remove projected events. A missing DB parses
all owning definitions, task links, and checkpoint JSON files before returning
the same status and then seeds the projection.

Warm and cold outputs must be semantically identical after excluding diagnostic
fields such as `state_source` and `projection_refreshed`. There is no bounded
heuristic fallback or partial result. If reconstruction is canceled or an
artifact cannot be read, the command fails visibly rather than returning weaker
scheduling output.

F03 performance gates are p95 under 250 ms for a repo owner with 1,000 events,
under 2 seconds for a 10-repo workspace owner with 10,000 events, and under 50
ms when the projection is fresh on the canonical CI runner. These are
acceptance targets, not semantic cutoffs. Progress moves to stderr after 250 ms
and names the reconstruction phase.

## CLI

```text
ds thread [<task-id|change-id>]
ds thread set <owner> <key> <target>... [--name <label>] [--after <key>]...
ds thread remove <owner> <key>
ds apply [<task-id|change-id>] --thread <key>
```

`ds thread` reports every ready/active lane and why other lanes wait. `set`
declaratively creates or replaces one definition after complete validation.
`remove` uses the history guards above. There is no duplicate
`ds workspace thread` command. `ds apply` remains the only prompt emitter and
routes a workspace target through its repo alias without changing directories
or weakening its packed context.

Thread state is derived as `ready`, `active`, `waiting`, `blocked`, or
`completed`; a target may additionally be `conflicted`. Improve, rework, block,
rollback, cancellation, and unresolved supersession do not satisfy a join.
Definition order controls presentation only, never priority.

Apply without a selector continues only for legacy linear tasks or exactly one
runnable thread. Explicit target application validates dependencies. `ds task
status` exposes a singular `next_target` only when one exists. `ds task next`
remains a hidden compatibility path and never chooses among ready lanes.

## Output contracts

Repo status:

```text
Task: checkout-redesign
Ready threads
  agent-a  A01  Add the server boundary
  human-a  A03  Confirm migration policy
Waiting threads
  both-a   A04  after agent-a, human-a
Run: ds apply checkout-redesign --thread agent-a
```

Workspace status qualifies targets:

```text
Change: COM-C014
Ready threads
  agent-a  api:A01  Add the API boundary
  human-a  web:B01  Confirm browser migration
Waiting threads
  both-a   api:A03  after agent-a, human-a
Run: ds apply COM-C014 --thread agent-a
```

JSON returns canonical owner and target objects, ordered thread arrays,
`ready_threads` as a set, wait/conflict reasons, and diagnostic `state_source`.
Consumers must not treat the first ready thread as a recommendation.

## Product boundaries

- Threads schedule existing targets; tasks own bounded plans and checkpoints.
- Workspaces remain optional umbrella coordination and change linking.
- Compose owns durable ADR/RFC/PRD documents, not execution scheduling.
- Prune may delete/rebuild projected SQLite rows but never durable definitions
  or events.
- Kleio may replicate definitions/events and offer shared views, but local CLI
  behavior remains complete while offline.

## Alternatives

- Global SQLite authority: transactionally convenient, but prune, rebuild,
  home isolation, or corruption would erase durable orchestration.
- One mutable `task.json`: inspectable, but a concurrency hotspot and the wrong
  owner for cross-repo lanes.
- Shared JSONL authority: stream-friendly, but still a shared append and Git
  merge hotspot.
- Task-local SQLite authority: transactional, but opaque to Git and awkward for
  graphs spanning repositories.
- Mandatory workspace ownership: one model, but needless ceremony for normal
  repo work.
- Remote Kleio authority: useful aggregation, but violates offline local-first
  operation.

## Rollout and release gates

The command is experimental in v1.4. F02 adds definitions and event authority;
F03 adds the projection and cold reconstruction; later slices integrate apply,
surface, concurrency, and dogfood.

Release is blocked if SQLite produces a better result than cold artifacts, if
a linked workspace task can accidentally create competing repo authority, or
if concurrent event heads are silently ordered rather than exposed.

<!-- devspecs: task=threaded-task-orchestration target=F01 -->
