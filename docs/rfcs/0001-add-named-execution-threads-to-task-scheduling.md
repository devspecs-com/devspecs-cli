# RFC: Add named execution threads to task scheduling

## Status

Accepted

## Summary

Add repo-local named execution threads to DevSpecs task manifests. A thread is
an ordered lane of existing slice IDs with optional dependencies on other
threads. The scheduler derives all thread state from slice checkpoints and can
therefore expose parallel work, joins, and later splits without creating a
second lifecycle system.

## Motivation

`ds task next` currently walks task slices in manifest order and returns one
global target. That model is useful for bounded serial work but cannot represent
two independent lanes, a target that waits for both, or another split after the
join. Humans and agents must carry that orchestration outside DevSpecs, which
makes the generated next recommendation misleading precisely when work becomes
architecturally interesting.

The v1.4 task surface already has the right sources of truth: slices define
bounded work, checkpoints define lifecycle state, `ds apply` emits one bounded
prompt, and the final track closeout reviews durable knowledge. Threads should
compose those mechanisms rather than replace them.

## Goals

- Represent serial lanes, parallel splits, joins, and later splits with a small
  deterministic manifest graph.
- Let humans and agents discover every currently runnable lane without an
  arbitrary global priority.
- Keep one-target prompts and checkpoint gates intact.
- Preserve legacy task behavior when no threads are configured.
- Make concurrent checkpoint writes loss-safe before advertising parallel use.

## Non-goals

- Launching, assigning, claiming, monitoring, or terminating agent processes.
- Adding thread-specific plans, results, checkpoints, stages, or decisions.
- Coordinating targets across different repo tasks; umbrella work remains a
  workspace concern.
- Replacing slice follow-ups or the once-per-track durable record closeout.

## Proposal

### Manifest model

```json
"threads": [
  {"key": "agent-a", "name": "Agent A", "targets": ["F02", "F03"]},
  {"key": "human-a", "name": "Human A", "targets": ["F04"]},
  {
    "key": "both-a",
    "name": "Both A",
    "targets": ["F05"],
    "after": ["agent-a", "human-a"]
  },
  {"key": "agent-b", "targets": ["F06"], "after": ["both-a"]},
  {"key": "human-b", "targets": ["F07"], "after": ["both-a"]}
]
```

Keys and target membership are unique. Dependencies must reference existing
threads, self-dependencies and cycles are invalid, and series closeout targets
cannot be assigned. A follow-up such as `F02-1` inherits `F02`'s thread and runs
before the lane advances.

Thread state is derived as `ready`, `active`, `waiting`, `blocked`, or
`completed`. Completion requires every lane target to end with an
advance-allowing gate. Improve, rework, block, and rollback remain unresolved;
they do not satisfy a join. Explicitly configured tasks report unassigned
nonterminal slices and refuse implicit scheduling rather than running them by
accident.

Manifest order controls presentation only; it is never a global priority.
Within one thread, target order is authoritative. A thread is `waiting` while
an `after` dependency is unresolved, `ready` when its dependencies allow
advance and its next target has not started, `active` when its next target has
started, `blocked` when that target has a non-advancing decision, and
`completed` when every target allows advance. A dependency blocked on improve,
rework, block, or rollback keeps downstream threads waiting and reports that
reason explicitly.

### CLI

```text
ds thread [task-id]
ds thread set <task-id> <key> <target>... [--name <label>] [--after <key>]...
ds thread remove <task-id> <key>
ds apply [task-id] --thread <key>
```

The status form reports every ready/active lane and why other lanes wait.
`set` declaratively creates or replaces one definition after full graph
validation. `remove` is limited to definitions without started or terminal
history. Thread keys such as `agent-a` are labels only; output must not imply
that DevSpecs owns an agent process.

Replacing a definition may reorder or reassign only targets without started or
terminal lifecycle history. Removing a thread never deletes targets or task
artifacts, and removal is rejected while another thread depends on it. These
guards keep graph edits from rewriting the meaning of recorded checkpoints.

`ds apply --thread` selects the next runnable target within one lane. Apply by
explicit target still validates thread dependencies. Apply without a selector
continues to work when a legacy task or exactly one runnable lane makes the
choice unambiguous.

Human status output follows this shape:

```text
Task: checkout-redesign

Ready threads
  agent-a  A01  Add the server boundary
  human-a  A03  Confirm migration policy

Waiting threads
  both-a   A05  after agent-a, human-a

Unassigned targets: none

Run: ds apply checkout-redesign --thread agent-a
```

JSON status exposes readiness as a set and preserves wait reasons:

```json
{
  "task_id": "checkout-redesign",
  "threads": [
    {
      "key": "agent-a",
      "name": "Agent A",
      "state": "ready",
      "targets": ["A01", "A02"],
      "next_target": "A01"
    },
    {
      "key": "human-a",
      "name": "Human A",
      "state": "ready",
      "targets": ["A03"],
      "next_target": "A03"
    },
    {
      "key": "both-a",
      "name": "Both A",
      "state": "waiting",
      "targets": ["A05"],
      "after": ["agent-a", "human-a"],
      "wait_reason": "waiting for agent-a, human-a"
    }
  ],
  "ready_threads": ["agent-a", "human-a"],
  "unassigned_targets": []
}
```

The JSON arrays follow manifest order for deterministic output. Consumers must
still treat `ready_threads` as a set, not as a recommendation to pick its first
member.

### Compatibility and ownership

Legacy manifests have an implicit linear lane and need no migration. `ds task
next` remains a hidden compatibility path for legacy or unambiguous tasks, but
must never pick arbitrarily among multiple ready threads. `ds task status`
includes thread summaries and exposes a singular `next_target` only when one
exists.

Reading a legacy manifest does not write an implicit thread back to disk.
Adding the first explicit definition switches that task to graph scheduling;
until all nonterminal implementation slices are assigned, status reports the
unassigned targets and implicit apply/next selection refuses to proceed. A
follow-up inherits its parent's thread without being copied into the manifest.
Series closeout remains outside every thread and becomes eligible only after
all implementation threads complete with advance-allowing gates.

Task inference follows the existing apply rule: `ds thread` may omit the task
ID only when the current repository has one unambiguous active task. Otherwise
it returns candidate task IDs and asks for an explicit selection.

Threads live only in the repo task's `task.json`. Workspace callers use the
existing `--repo` routing boundary; no `ds workspace thread` is added. Compose
documents and prune/index storage retain their existing ownership contracts.

### Concurrent mutation

Parallel lanes make concurrent checkpoints expected behavior. All task
read-modify-write operations therefore acquire a crash-safe cross-process lease
keyed by canonical task workspace path, re-read the manifest under that lease,
and publish JSON atomically. Expensive evidence collection stays outside the
critical section. The lock lives in local DevSpecs state, not in the repository.

## Alternatives

- Keep one global priority or `next` pointer: simple, but it hides legitimately
  parallel work and cannot express joins.
- Add dependencies directly to every slice: expressive, but verbose for common
  lanes and less readable for humans returning to a task.
- Use separate task tracks for every lane: avoids a schema change but loses one
  shared closeout and makes joins external again.
- Put thread orchestration under `ds workspace`: incorrect for parallel work
  inside one repo task and would duplicate repo task lifecycle semantics.
- Add assignee/claim/heartbeat fields: useful for an orchestration service, but
  misleading in a local CLI that does not own agent processes.

## Risks and rollout

The main risk is promising parallel use before shared manifest mutation is
safe. Cross-process lost-update tests are a release gate, not follow-up polish.
The second risk is surface expansion: status-by-default, `set`, and guarded
`remove` are the maximum proposed management surface, while `ds apply` remains
the only prompt command.

Rollout is additive for manifests and initially documented as experimental.
Legacy tasks and scripts retain linear behavior. `ds task next` is hidden only
after compatibility and ambiguity tests pass. Rollback consists of leaving the
optional `threads` field unread; slice and checkpoint history remains valid.

## Resolved release decisions

- v1.4 includes guarded `remove`. It is useful for correcting an unstarted
  graph and cannot erase or reinterpret started or terminal history.
- The command and documentation label threads experimental in v1.4. The
  optional manifest shape remains additive and forward-compatible.

<!-- devspecs: task=threaded-task-orchestration target=F01 -->
