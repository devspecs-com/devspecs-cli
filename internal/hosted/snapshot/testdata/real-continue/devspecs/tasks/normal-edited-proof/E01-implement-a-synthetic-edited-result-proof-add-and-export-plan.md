# Task normal-edited-proof E01 Plan

## Goal
Implement a synthetic edited-result proof: add and export subtract(a, b) in src/sum.js while preserving sum. Add one descriptive Node test in test/sum.test.js proving subtract(5, 3) equals 2. Run npm test. Do not touch devspecs, .devspecs, package metadata, or any other file. The provider commits the implementation result only; the caller alone manages task checkpoints.

## Description
Create a bounded implementation slice for `Implement a synthetic edited-result proof: add and export subtract(a, b) in src/sum.js while preserving sum. Add one descriptive Node test in test/sum.test.js proving subtract(5, 3) equals 2. Run npm test. Do not touch devspecs, .devspecs, package metadata, or any other file. The provider commits the implementation result only; the caller alone manages task checkpoints.`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Resources
- `E00-index.md`
- `E01-implement-a-synthetic-edited-result-proof-add-and-export-result.md`
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

## Tasks
- [ ] Inspect the predicted primary files.
- [ ] Inspect same-package, same-stem, or receipt-related tests.
- [ ] Refine the slice if context is incomplete.
- [ ] Implement the smallest useful change.
- [ ] Run focused validation.
- [ ] Update `E01-implement-a-synthetic-edited-result-proof-add-and-export-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
- Block: external input or a missing prerequisite prevents useful progress.
