# Task task-redundancy-checking O02 Plan

## Goal
Plan task inventory and creation-time redundancy checks.

## Claim
Redundancy warnings are most useful in two places: when a human/agent lists open work, and when a new task would duplicate an existing non-terminal slice.

## Candidate Surfaces
- `ds task list`: show overlap groups among open plans by default or behind `--overlaps`.
- `ds task "goal"`: after packing, warn when the query and planned files overlap non-terminal slices.
- `ds apply next`: warn if the resolved next target has a route conflict or overlaps another open target.
- `ds task show <id>`: show related open/superseded tasks as context, not as instructions.

## Minimal Data Model
Keep the first implementation local and task-manifest driven:

- task ID, series, slice ID, title, stage, decision
- plan/result path
- planned edit paths if extractable from plan sections
- packed source/test/doc paths from task manifest
- recent checkpoint evidence: files read, files edited, missed files, noise files, next target
- optional body-derived rare terms from title, query, and headings

## Output Shape
Human output should group by reason:

```text
Possible overlapping open work (2)
- H02 ... [stage: planned]
- M02 ... [stage: packed]
Reason: shared planned surface `internal/commands/task.go`; shared terms: task, list, open
Next: inspect with `ds task show H02`; continue only if this is intentionally separate.
```

JSON output should expose stable fields:

- `overlap_id`
- `severity`
- `reason`
- `evidence`
- `targets`
- `recommended_action`

## Integration Constraints
- Do not require a full repo scan for every warning.
- Do not run expensive similarity checks by default.
- Do not hide task creation behind an interactive prompt.
- Keep warnings suppressible in future if they become noisy.
- Preserve explicit route semantics from N track; route conflicts are a specific overlap family, not the whole model.

## Acceptance Checks
- [ ] The plan names the first command surface to implement.
- [ ] JSON and human output fields are specified.
- [ ] The implementation can run from task manifests before using broad artifact search.
- [ ] Warnings distinguish open work from closed/superseded work.
- [ ] `ds task list` and `ds task create` tradeoffs are explicit.

## Decision Gates
- Promote if the first implementation surface is clear and low-risk.
- Improve if both inventory and creation-time warnings are useful but need sequencing.
- Rework if the plan requires broad search or scan work before any warning can work.
- Rollback if the UX becomes a blocking flow.
- Block if task manifests lack the data needed for a useful first detector.
