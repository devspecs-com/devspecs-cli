# Task task-redundancy-checking O01 Plan

## Goal
Define overlap signals and non-blocking warning UX.

## Claim
DevSpecs can reduce task drift by flagging likely redundant open work without blocking task creation or pretending to automatically merge plans.

## Scope
Define the first detector contract and warning shape only. Implementation can wait until O02/O03 validate that the signal is good enough.

## Signals To Specify
- Open/unimplemented task slices with overlapping planned edit paths.
- Open/unimplemented task slices with rare title/query term overlap.
- Repeated source/test packs across task workspaces where neither slice is completed.
- New task query overlaps an existing non-terminal slice.
- Prior checkpoint missed/noise/learning evidence resurfaces in a new plan.
- Terminal states (`complete`, `cancel`, `supersede`, `rollback`) demote warnings unless the new work explicitly reopens them.

## Warning Principles
- Warnings must name the existing task/slice IDs, titles, state, and evidence.
- Warnings should be advisory: `possible overlap`, not `duplicate`.
- The default action should be to inspect or link the existing task, not to stop.
- The user/agent should get choices: continue, link, supersede, split, ignore.
- Warnings should be short enough to fit inside `ds task` and `ds apply` output without becoming a wall of text.

## Out Of Scope
- Auto-merging task files.
- Auto-superseding older work.
- Hidden vector similarity with no receipts.
- Broad theme overlap without overlapping implementation intent.
- Warnings for closed work unless the query clearly reopens the same change.

## Acceptance Checks
- [ ] The detector has at least three deterministic signal families.
- [ ] The warning UX explains why the overlap was flagged.
- [ ] The plan distinguishes overlap, redundancy, supersession, and historical context.
- [ ] False-positive risks are explicit.
- [ ] The next slice can choose where the warnings appear in the CLI.

## Decision Gates
- Promote if the warning contract is understandable enough for a future implementation slice.
- Improve if the concepts are right but too broad for low-noise CLI output.
- Rework if the model depends on opaque semantic similarity before deterministic signals.
- Rollback if it would make task creation feel punitive.
- Block if lifecycle states are too weak to classify open versus closed work.
