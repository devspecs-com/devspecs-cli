# Task scanless-workflow-ux A01 Plan

## Goal
Fold ds scan into workflow commands so onboarding starts with map, find, and task instead of a required manual scan

## Description
Launch UX polish: make the happy path start with workflow commands that already refresh/index as needed. Keep `ds scan` as a visible manual refresh/rebuild command, but stop presenting it as a required onboarding step.

## Resources
- `A00-index.md`
- `A01-fold-ds-scan-into-workflow-commands-so-onboardin-result.md`
- `task.json`
- `internal/commands/task.go`
- `internal/commands/map.go`
- `internal/scan/scan.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/find_pack_companions.go`
- `internal/commands/scan.go`
- `internal/commands/tldr.go`
- `internal/retrieval/retrieval.go`

## Starting Context
### Files to Inspect First
- `internal/commands/task.go`
- `internal/commands/map.go`
- `internal/scan/scan.go`
- `internal/commands/task_evaluate.go`
- `internal/commands/find_pack_companions.go`
- `internal/commands/scan.go`
- `internal/commands/tldr.go`
- `internal/retrieval/retrieval.go`
- `internal/commands/eval.go`
- `internal/evalharness/eval.go`
- `internal/commands/find.go`
- `internal/commands/find_pack.go`

### Tests to Inspect First
- `internal/commands/map_test.go#L1440`
- `internal/scan/scan_test.go#L248`
- `internal/commands/task_test.go#L1903`
- `internal/commands/eval_test.go#L619`
- `internal/commands/find_pack_companions_test.go#L61`
- `internal/commands/find_pack_test.go#L206`
- `internal/commands/init_test.go#L13`
- `internal/scan/scan_timestamps_test.go#L18`
- `internal/commands/tldr_test.go#L34`
- `internal/retrieval/retrieval_test.go#L998`
- `internal/commands/retrieval_bridge_test.go#L63`

## Expected Change Surface
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- `internal/commands/init.go`
- `internal/commands/init_test.go`
- `internal/commands/resume.go`
- `internal/commands/v01_test.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `README.md`

## Out-of-Scope Areas
- Replanning the whole thread unless evidence says this slice should split or be superseded.
- Broad pack-ranking changes unless they are necessary for this task.
- Treating the generated context as complete without verification.
- Changing scan internals, retrieval ranking, pack scoring, or task packing defaults.
- Removing `ds scan`.

## Risks
- Pack completeness is not high; verify the working set before editing.

## Success Criteria
- [ ] `ds init` points users to `ds map`, `ds find`, and `ds task quick`, with `ds scan` framed as manual refresh.
- [ ] `ds tldr` workflow examples no longer require `ds scan` before `map`, `find`, or `task`.
- [ ] `ds map` supports `--no-refresh`, matching other read/workflow commands.
- [ ] Map caveats no longer tell users to run `ds scan` before suggested `ds find --pack` commands.
- [ ] README brownfield quickstart starts with workflow commands, not manual scan.
- [ ] Focused tests cover init output, TLDR output, and map no-refresh behavior.

## Tasks
- [ ] Update init/resume copy.
- [ ] Update TLDR command examples and assertions.
- [ ] Add `map --no-refresh` and tests.
- [ ] Update README brownfield/core command framing.
- [ ] Run focused tests and full `go test ./...`.
- [ ] Record a checkpoint and sync task artifacts.

## Decision Gates
- Promote: the workspace was useful enough and misses are actionable.
- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.
- Rework: task workspace feels like planning overhead or fails to capture useful evidence.
- Rollback: workspace creates false confidence or worsens agent performance.
