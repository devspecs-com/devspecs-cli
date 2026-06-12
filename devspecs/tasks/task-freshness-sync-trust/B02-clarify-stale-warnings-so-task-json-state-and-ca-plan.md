# Task task-freshness-sync-trust B02 Plan

## Goal
Clarify stale warnings so task.json state and captured artifact freshness are reported separately

## Description
Create a bounded planning slice for `Task freshness and sync trust repair`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Dogfood Scenario
A user manually corrected a generated `Q00` after `ds task sync`. Afterwards, `ds task status` reported `Q00` as stale even though `task.json` still had the right slice state. Running `ds task sync --help` showed no refresh-only mode, so the user considered letting the CLI own the index freshness even if that meant losing the manual readability patch.

The CLI should distinguish:
- lifecycle state freshness from `task.json`
- captured artifact freshness in the DevSpecs index
- authored Markdown readability edits that should not be overwritten

## Reproduced Current Behavior
On 2026-06-12, running current-source `ds task sync <id> --dir devspecs/tasks --json` after editing generated index docs successfully indexed the edited paths and did not rewrite Markdown. However, the JSON response still returned `artifact_freshness` entries for the edited files with the reason `task artifact changed after the task state was last captured; run ds task sync`.

That means the command can appear to tell the user to run the command they just ran. The likely product bug is that captured artifact freshness is still coupled to task state `updated_at`, rather than a separate artifact capture timestamp or post-sync freshness state.

## Resources
- `B00-index.md`
- `B02-clarify-stale-warnings-so-task-json-state-and-ca-result.md`
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
- On-disk paths matched the task but were not indexed: Inspect the warned files or refresh the index before trusting missing context. Evidence: `internal/commands/freshness_test.go` - on-disk path matched task terms but was not in the indexed candidate set: freshness; `internal/commands/task_test.go` - on-disk path matched task terms but was not in the indexed candidate set: task.

## Success Criteria
- [ ] `ds task status` can explain whether the problem is lifecycle state, captured index freshness, or both.
- [ ] The warning copy gives a clear next command without implying manual task docs must be discarded.
- [ ] A refresh-only UX is specified or explicitly rejected with rationale.
- [ ] The next slice recommendation is promote, improve, rework, rollback, or block.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `B02-clarify-stale-warnings-so-task-json-state-and-ca-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
