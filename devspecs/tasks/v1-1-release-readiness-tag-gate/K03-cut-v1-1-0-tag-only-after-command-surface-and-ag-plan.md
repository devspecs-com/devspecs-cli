# Task v1-1-release-readiness-tag-gate K03 Plan

## Goal
Cut v1.1.0 tag only after command surface and agent tooling gates pass

## Description
Create a bounded implementation slice for `v1.1 release readiness and tag gate`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Slice-Specific Scope
- Cut `v1.1.0` only after I/J implementation work and K01/K02 release checks pass.
- Verify the public command list has no accidental legacy/confusing launch story commands.
- Verify CI is green before tagging.
- Prepare release notes that acknowledge the command realignment without creating semver confusion.
- Do not tag from this slice if architecture map or apply is still ambiguous without a beta marker.

## Resources
- `K00-index.md`
- `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-result.md`
- `task.json`

## Starting Context
### Files to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

### Tests to Inspect First
- No pack-ranked files. Verify checkpoint leads below or search before editing.

## Expected Change Surface
- Unknown. Identify the primary file before editing.

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.

## Risks
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Pack completeness is not high; verify the working set before editing.

## Success Criteria
- [ ] Primary implementation surface is verified before edits.
- [ ] Relevant tests are found or the test-surface miss is recorded.
- [ ] Changes stay inside the bounded slice.
- [ ] A checkpoint records actual files, tests, misses, noise, and decision.
- [ ] `git tag v1.1.0` is created only from a green, documented release commit.
- [ ] The result artifact records commit SHA, tag SHA, CI status, and release notes location.

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `K03-cut-v1-1-0-tag-only-after-command-surface-and-ag-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
