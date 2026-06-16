# Task v1-1-agent-tooling-apply-loop J03 Result

## Summary
- Target: `J03` - Add ds apply prompt surface for next or explicit slice identifiers
- Outcome: Implemented and validated. `ds apply` now emits the bounded one-slice agent prompt for `next`, a task id, a slice/iteration id, or a task id plus `--target`.

## Changed Files
- `internal/commands/apply.go` - added the prompt-only top-level `ds apply` command, target resolution, `next` disambiguation, series/index handling, JSON output, and privacy-safe telemetry.
- `internal/commands/apply_test.go` - covered `next`, explicit target resolution, task+target resolution, series index resolution, no lifecycle mutation, and ambiguity errors.
- `cmd/ds/main.go` - registered `ds apply` and placed it in the root help workflow.
- `cmd/ds/main_test.go` - covered command registration and root help placement.

## Tests
- `go test ./internal/commands -run "TestApply|TestTask_BoundaryPrimitivesResolveOneTarget|TestTask_TargetAddressing" -count=1`
- `go test ./internal/commands -run TestApply -count=1`
- `go test ./internal/commands -count=1 -timeout=5m`
- `go test ./cmd/ds -run "TestRootCmd_ApplyRegistered|TestRootCmd_PublicHelpHidesInternalCommands" -count=1`
- `go test ./cmd/ds -count=1`
- Smoke: `.devspecs/bin/ds.exe apply J03 --json` resolved `v1-1-agent-tooling-apply-loop` target `J03` and included the completion contract.
- Smoke: `.devspecs/bin/ds.exe apply v1-1-agent-tooling-apply-loop --target J03` emitted a human prompt bounded to `J03`.

## Decision
- Promote. The command is launch-useful, prompt-only, and reuses the existing task target resolver/prompt renderer instead of adding orchestration state.

## Follow-up
- J04 should focus on adapter/docs polish now that `ds apply` exists. It should verify the generated `/ds-apply` and `$ds-apply` text no longer needs fallback language beyond compatibility with older binaries.
- A future slice can add tool-specific colon aliases only where a tool supports aliases without Windows-hostile filenames.

## References
- `J00-index.md`
- `J03-add-ds-apply-prompt-surface-for-next-or-explicit-plan.md`

## Checkpoints
- Use `ds task checkpoint v1-1-agent-tooling-apply-loop --target J03` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T16:27:52Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-162752-validated.md`
- Structured Evidence: `checkpoints/20260616-162752-validated.json`
- Note: ds apply emits a bounded prompt and intentionally does not mark targets started or advance lifecycle state.
- What changed: Added prompt-only ds apply for next, task ids, explicit slice identifiers, and task --target resolution using the existing task prompt renderer.
- Evidence for decision: 4 file(s) edited; 5 test command(s)
- What remains: next target J04
- Next iteration: J04 with decision -
- Files edited:
  - `internal/commands/apply.go`
  - `internal/commands/apply_test.go`
  - `cmd/ds/main.go`
  - `cmd/ds/main_test.go`
- Tests run:
  - `go test ./internal/commands -run TestApply|TestTask_BoundaryPrimitivesResolveOneTarget|TestTask_TargetAddressing -count=1`
  - `go test ./internal/commands -run TestApply -count=1`
  - `go test ./internal/commands -count=1 -timeout=5m`
  - `go test ./cmd/ds -run TestRootCmd_ApplyRegistered|TestRootCmd_PublicHelpHidesInternalCommands -count=1`
  - `go test ./cmd/ds -count=1`
