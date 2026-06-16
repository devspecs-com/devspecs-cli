# Task v1-1-command-surface-realignment I02 Result

## Summary
- Target: `I02` - Reclaim ds map for architecture and system boundary output using boundary evidence
- Outcome: promoted `ds map` to the stable architecture/system boundary surface. `ds recent` remains the recent-activity command, while hidden `--experimental-boundaries` and `--recent` flags stay as compatibility paths.

## Changed Files
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`

## Tests
- `go test ./internal/commands -run TestMapTextHidesReviewerDiagnosticsByDefault|TestMapJSONSchemaIsAgentReadable|TestRecentCommandShowsRecentTopics|TestMapRecentFlagRemainsCompatibilityPath|TestMapAutoScanLeavesUsableIndexForFindPack|TestMapNoRefreshSkipsAutoScan|TestMapJSONAutoScanKeepsStdoutJSON|TestTLDR_HumanOutputGroupsWorkflows -count=1`
- `go run ./cmd/ds map --no-refresh --max-areas 3`

## Decision
- Promote to `I03`.

## Follow-up
- `I03`: add CLI/tooling initialization for agent slash-command and skill adapters.

## References
- `I00-index.md`
- `I02-reclaim-ds-map-for-architecture-and-system-bound-plan.md`

## Checkpoints
- Use `ds task checkpoint v1-1-command-surface-realignment --target I02` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T09:07:59Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-090759-validated.md`
- Structured Evidence: `checkpoints/20260616-090759-validated.json`
- Files read:
  - `internal/commands/map.go`
- Files edited:
  - `internal/commands/map.go`
  - `internal/commands/map_test.go`
  - `internal/commands/tldr.go`
  - `internal/commands/tldr_test.go`
  - `README.md`
  - `TASK_WORKFLOW_EXAMPLE.md`
- Tests run:
  - `go test ./internal/commands -run TestMapTextHidesReviewerDiagnosticsByDefault|TestMapJSONSchemaIsAgentReadable|TestRecentCommandShowsRecentTopics|TestMapRecentFlagRemainsCompatibilityPath|TestMapAutoScanLeavesUsableIndexForFindPack|TestMapNoRefreshSkipsAutoScan|TestMapJSONAutoScanKeepsStdoutJSON|TestTLDR_HumanOutputGroupsWorkflows -count=1`
  - `go run ./cmd/ds map --no-refresh --max-areas 3`
