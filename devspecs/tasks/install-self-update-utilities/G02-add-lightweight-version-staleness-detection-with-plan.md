# Task install-self-update-utilities G02 Plan

## Goal
Add lightweight version staleness detection without checking on every command

## Description
Create a bounded planning slice for `Install and self-update utilities`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Resources
- `G00-index.md`
- `G02-add-lightweight-version-staleness-detection-with-result.md`
- `task.json`

## Product Shape
Do not check latest version on every command. Start with explicit checks through `ds update`, and maybe `ds version --check`. If a cache is added, keep it local, timestamped, and quiet unless the user asks.

## Starting Context
### Evidence to Review
- No likely primary files were found; identify the first artifact from the repo and task goal.

### Test or Evaluation Signals
- No likely tests were found; define the first useful validation signal.

## Expected Change Surface
- Planning artifacts, acceptance checks, interface notes, eval cards, or test design.
- Implementation code only if the slice explicitly narrows to one low-risk first artifact.

## Out-of-Scope Areas
- Treating a greenfield planning slice as permission to implement the full thread.
- Broad retrieval or pack-ranking changes unless the slice is explicitly about DevSpecs itself.
- Assuming the generated context is complete without recording verification.

## Risks
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Pack completeness is not high; verify the working set before committing to implementation scope.

## Success Criteria
- [ ] No network/version check runs during normal `find`, `task`, `map`, or `tldr` usage.
- [ ] Explicit check path handles offline/network failure gracefully.
- [ ] Cache, if any, is local and has a clear TTL.
- [ ] Output distinguishes current, stale, unknown, and development builds.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `G02-add-lightweight-version-staleness-detection-with-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
