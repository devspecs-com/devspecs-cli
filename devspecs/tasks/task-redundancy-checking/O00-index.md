# Task task-redundancy-checking

## Task
Task redundancy checking: flag overlapping unimplemented plans and repeated planned changes across tracks before agents create or follow duplicate work.

## Status
packed

## Series
O

## Profile
code-change

## Created At
2026-06-17T12:47:30Z

## Original Query
Task redundancy checking: flag overlapping unimplemented plans and repeated planned changes across tracks before agents create or follow duplicate work

## Product Read
Dogfooding keeps surfacing the same failure mode: as tracks accumulate, we can accidentally re-plan work that already exists elsewhere but has not been implemented yet. This is different from normal search noise. It is task-system drift:

- two open slices describe the same intended change
- a new task recreates an unimplemented older plan
- a route loop says one target is next, while another new plan quietly duplicates it
- agents see the duplicate and treat it as fresh authority instead of resolving the existing task

The product value is not automatic deduplication. The useful first version should make overlap visible early enough that humans and agents can choose whether to continue, merge, supersede, or link.

## Priority
Backlog / post-v1.1 unless repeated dogfood pain makes it launch-critical. This should not interrupt the current v1.1 route.

## Relationship To Other Tracks
- M track (`task-list-inventory-ux`) owns open-plan inventory UX; O can feed warning data into that surface.
- N track (`cross-track-route-loops`) owns ordered route memory; O should warn when new plans conflict with or duplicate route steps.
- J track (`agent tooling + apply`) should eventually surface redundancy warnings in slash-command prompts, but not own the detector.
- F/I diagnostic work may improve the underlying ranking signals, but O should stay task-workspace aware.

## Resources
- `task.json`
- `O01-define-overlap-signals-and-non-blocking-warning-plan.md`
- `O01-define-overlap-signals-and-non-blocking-warning-result.md`
- `O02-plan-task-inventory-and-creation-time-redundancy-plan.md`
- `O02-plan-task-inventory-and-creation-time-redundancy-result.md`
- `O03-evaluate-redundancy-detection-against-dogfood-ta-plan.md`
- `O03-evaluate-redundancy-detection-against-dogfood-ta-result.md`

## Task Slices
- O01: Define overlap signals and non-blocking warning UX. Plan: `O01-define-overlap-signals-and-non-blocking-warning-plan.md`. Result: `O01-define-overlap-signals-and-non-blocking-warning-result.md`.
- O02: Plan task inventory and creation-time redundancy checks. Plan: `O02-plan-task-inventory-and-creation-time-redundancy-plan.md`. Result: `O02-plan-task-inventory-and-creation-time-redundancy-result.md`.
- O03: Evaluate redundancy detection against dogfood task corpus. Plan: `O03-evaluate-redundancy-detection-against-dogfood-ta-plan.md`. Result: `O03-evaluate-redundancy-detection-against-dogfood-ta-result.md`.

## Candidate Signals
Start with deterministic, explainable signals before adding semantic similarity:

- shared planned edit paths across open or unimplemented slices
- shared task title nouns and rare terms across open slices
- same route or series target referenced by multiple active tracks
- same source/test pairs packed into multiple unclosed task workspaces
- same checkpoint learnings or missed files resurfacing as new tasks
- explicit `supersede`, `blocked`, `validated/promote`, or `completed` states that should demote false alarms

## Warning UX
Warnings should be advisory and scoped:

```text
Possible overlapping open work:
- H02 "..."
- O02 "..."

Why: shared planned file `internal/commands/task.go`, shared phrase "task inventory", both unimplemented.
Try: ds task show H02
Decision: continue, link, supersede, split, or ignore
```

Do not block task creation by default. False positives would be more damaging than a quiet advisory miss.

## Non-Goals
- Do not invent a full task-merge workflow in the first pass.
- Do not auto-close, auto-supersede, or rewrite existing plans.
- Do not make semantic similarity opaque or vector-only.
- Do not warn on completed historical work unless the new plan explicitly appears to regress or reopen it.
- Do not treat broad theme overlap as redundancy; implementation intent must overlap.

## Decision Gates
- Promote if the warning model catches real duplicate open work with readable evidence and low ceremony.
- Improve if it finds useful overlaps but needs better state filtering or fewer false positives.
- Rework if warnings are mostly broad-topic noise.
- Rollback if warnings discourage valid parallel tracks or make task creation feel brittle.
- Block if task lifecycle state is insufficient to distinguish open, closed, superseded, and implemented work.

## Known Unknowns
- Whether task manifests alone are enough, or whether indexed artifact bodies are required.
- Whether overlap should run during `ds task`, `ds task list`, `ds apply`, or only via an explicit diagnostic command first.
- How aggressive warnings should be when plans share files but have different stages or route positions.
- Whether the detector should understand non-task intent artifacts such as PRDs, RFCs, ADRs, and OpenSpec changes.
