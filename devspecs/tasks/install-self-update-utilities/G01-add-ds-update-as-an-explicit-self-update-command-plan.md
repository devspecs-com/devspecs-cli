# Task install-self-update-utilities G01 Plan

## Goal
Add ds update as an explicit self-update command with package-manager-aware guidance

## Description
Create a bounded planning slice for `Install and self-update utilities`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.

## Resources
- `G00-index.md`
- `G01-add-ds-update-as-an-explicit-self-update-command-result.md`
- `task.json`

## Product Shape
`ds update` should first answer: installed version, latest known version if check is available, likely install source, and the recommended upgrade command. Running the upgrade can be a follow-up option only for clearly supported managers; guidance-only is acceptable for v1.

Candidate outputs:
- Homebrew: `brew update && brew upgrade devspecs-com/tap/devspecs`
- Scoop: `scoop update devspecs`
- Go install: `go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest`
- Script/manual install: print download/install-script guidance and restart-shell note

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
- [ ] `ds update --help` explains what it can and cannot update.
- [ ] The command is safe by default and can operate as guidance-only.
- [ ] Homebrew, Scoop, Go install, and script/manual installs have clear messages.
- [ ] Tests cover install-source detection without requiring network or real package managers.

## Tasks
- [ ] Review the task index and any likely evidence files.
- [ ] Define the first claim, interface, adapter, data model, or evaluation target.
- [ ] Draft the smallest useful planning artifact for that target.
- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.
- [ ] Update `G01-add-ds-update-as-an-explicit-self-update-command-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the planning slice gives the next agent a bounded, useful unit of work.
- Improve: the slice is directionally useful but needs another planning iteration.
- Rework: the plan chose the wrong claim, artifact, or evaluation target.
- Rollback: the scaffold added noise or false confidence.
