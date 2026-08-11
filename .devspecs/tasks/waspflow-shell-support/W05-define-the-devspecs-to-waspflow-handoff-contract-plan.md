# Task waspflow-shell-support W05 Plan

## Goal
Define the DevSpecs-to-Waspflow handoff contract and decide whether direct product support is warranted.

## Existing Surfaces To Test First
- DevSpecs task JSON, slice plan/result artifacts, `task show/status/next`, and explicit checkpoints.
- Waspflow `spawn`, `exec`, `--cwd`, `--isolate`, `--report`, `verify`, `reap`, and append-only receipts.
- Waspflow's planned MCP adapter is not implemented and is not a prerequisite.

## Questions
- Can an orchestrator pass one DevSpecs slice and its exact context to a Waspflow lane without lossy prompt reconstruction?
- Which system owns task state, lane state, verification evidence, and the final promotion decision?
- How are repository/worktree identity and artifact paths represented across an isolated lane?
- What cancellation, timeout, retry, and concurrent-index behavior is required?
- Is a stable JSON/file contract sufficient, or is one thin adapter justified?

## Work
- Write a short decision record with sequence diagrams for task-to-lane and lane-to-checkpoint flows.
- Specify versioned input/output examples using existing fields where possible.
- Threat-model stale task context, wrong worktree, missing report, failed verification, duplicate receipts, and partial process exit.
- Perform a CLI surface review against the current v1.4.0 release-candidate command inventory.
- Choose one outcome: existing composition, Waspflow-owned adapter, DevSpecs-owned adapter, shared MCP contract, or no integration.

## Non-Negotiable Semantics
- A Waspflow `verified` result can be attached as evidence but cannot automatically promote a DevSpecs slice.
- A DevSpecs task is not a Waspflow lane, and their lifecycle/status values must not be conflated.
- The handoff must identify the logical repository and effective worktree explicitly.
- No hidden re-index, warm-only enrichment, or silent fallback to docs-only context.

## Acceptance
- One reviewed decision record answers ownership, trust, identity, lifecycle, and failure semantics.
- Existing-surface composition is attempted before proposing a new command.
- Any proposed surface has a machine-readable schema, compatibility story, and deletion criterion.
- W06 has a bounded experiment or is explicitly skipped because direct support has no demonstrated value.

## Decision Gates
- Promote: a thin contract is justified and testable.
- Improve: composition works but one ownership/failure rule remains unclear.
- Supersede W06: existing commands and files already compose cleanly.
- Rework: the proposal duplicates either product's lifecycle or creates hidden authority.
