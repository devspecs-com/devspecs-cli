# Task evidence-lanes-domain-extractors D02 Plan

## Goal
Add extractor contract for domain facts concepts mentions and artifact edges

## Description
Create a bounded planning slice for `Evidence lanes and domain extractors`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Resources
- `D00-index.md`
- `D02-add-extractor-contract-for-domain-facts-concepts-result.md`
- `task.json`

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
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence; `internal/retrieval/pack_negative_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence.

## Success Criteria
- [ ] The slice states the product or engineering claim being settled.
- [ ] Interfaces, adapters, data model, or evaluation shape are named at the right level of detail.
- [ ] Known unknowns and assumptions are recorded.
- [ ] The next slice recommendation is promote, improve, rework, rollback, or block.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `D02-add-extractor-contract-for-domain-facts-concepts-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
