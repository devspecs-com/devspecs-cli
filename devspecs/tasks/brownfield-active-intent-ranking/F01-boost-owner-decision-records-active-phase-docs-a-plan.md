# Task brownfield-active-intent-ranking F01 Plan

## Goal
Boost owner decision records active phase docs and Status next plans above blocked or superseded epics

## Description
Create a bounded planning slice for `Brownfield active intent ranking and scoped find packs`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Resources
- `F00-index.md`
- `F01-boost-owner-decision-records-active-phase-docs-a-result.md`
- `task.json`

## Dogfood Scenario
ScopeLab had already chosen Epoch 4, but before EV artifacts existed, `ds find "epoch 4 external validity bridge"` pointed agents at blocked D4.2-era work. Current decision memos, `north_star` active-phase docs, and `Status: next` plans need stronger authority than blocked/superseded historical plans.

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
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before committing to implementation scope.
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find; `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find.

## Success Criteria
- [ ] Owner decision records and active-phase docs get explicit positive authority signals.
- [ ] Blocked, superseded, closed, or stale epics are demoted when current decision docs exist.
- [ ] Fixtures cover before-current-artifact and after-current-artifact behavior.
- [ ] Find output still labels historical context when it is useful but non-operational.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `F01-boost-owner-decision-records-active-phase-docs-a-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
