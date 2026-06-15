# Task brownfield-active-intent-ranking F02 Plan

## Goal
Tighten exact plan ID and track ID find packs so direct neighbors beat tangential historical plans

## Description
Create a bounded planning slice for `Brownfield active intent ranking and scoped find packs`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Resources
- `F00-index.md`
- `F02-tighten-exact-plan-id-and-track-id-find-packs-so-result.md`
- `task.json`

## Dogfood Scenario
After EV00/EV-R1 artifacts existed, `ds find` surfaced useful material but still included PLAN-008.1 synthetic repo world as historically related noise. For queries containing exact plan IDs or track IDs such as `EV-R1`, direct artifacts and direct neighbors should dominate the pack.

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
- [ ] Exact plan/track ID queries prioritize exact title/path/body matches and explicitly linked neighbors.
- [ ] Tangential historical plans are capped or moved to clearly downgraded historical/supporting context.
- [ ] The pack remains useful when the exact ID is absent, but does not overfit every query into ID mode.
- [ ] Fixtures include a recognizable `EV-R1` style query with a tempting but non-operational historical analog.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `F02-tighten-exact-plan-id-and-track-id-find-packs-so-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
