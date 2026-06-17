# Task cross-track-route-loops N03 Plan

## Goal
Decide when route loops graduate into CLI primitives

## Description
Create a bounded implementation slice for `Cross-track route loop placeholder: preserve ordered multi-track release routes, repair slices, smoke reruns, and later route views without building full orchestration yet`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.

## Resources
- `N00-index.md`
- `N03-decide-when-route-loops-graduate-into-cli-primit-result.md`
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
- Prior checkpoint recorded distracting context: Keep that family as reference-only unless this task verifies it. Evidence: task v1-1-release-readiness-tag-gate checkpoint cp_20260617T121918Z_k01_validated called distracting `ds task progress on stderr appears as NativeCommandError when PowerShell captures combined streams`.

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
- [ ] Update `N03-decide-when-route-loops-graduate-into-cli-primit-result.md` or run `ds task checkpoint`.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
- Block: external input or a missing prerequisite prevents useful progress.
