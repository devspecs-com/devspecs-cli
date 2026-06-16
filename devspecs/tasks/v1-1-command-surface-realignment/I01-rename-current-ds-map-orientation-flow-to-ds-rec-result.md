# Task v1-1-command-surface-realignment I01 Result

## Summary
- Target: `I01` - Rename current ds map orientation flow to ds recent with compatibility and docs
- Outcome: `ds recent` is now the first-class recent-activity orientation command; `ds map --recent` remains as a hidden/deprecated compatibility path.

## Changed Files
- `cmd/ds/main.go`
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `internal/commands/init.go`
- `internal/commands/init_test.go`
- `internal/commands/resume.go`
- `internal/commands/v01_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`

## Tests
- `go test ./internal/commands -run "TestRecentCommandShowsRecentTopics|TestMapRecentFlagRemainsCompatibilityPath|TestMapRecentTextAvoidsTaskStatusClaims|TestTLDR_HumanOutputGroupsWorkflows|TestInit_CreatesRepoConfig|TestResume_EmptyRepo" -count=1`
- `go run ./cmd/ds --help`
- `go run ./cmd/ds recent --no-refresh --max-areas 2`
- `go run ./cmd/ds map --help`

## Decision
- Promote

## Follow-up
- Continue to `I02`: reclaim `ds map` for architecture/system boundary output or place that surface under `ds beta map` if confidence is not high enough.

## References
- `I00-index.md`
- `I01-rename-current-ds-map-orientation-flow-to-ds-rec-plan.md`

## Checkpoints
- Use `ds task checkpoint v1-1-command-surface-realignment --target I01` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T08:47:31Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-084731-validated.md`
- Structured Evidence: `checkpoints/20260616-084731-validated.json`
- Files read:
  - `internal/commands/map.go`
  - `internal/commands/map_test.go`
  - `internal/commands/tldr.go`
  - `README.md`
- Files edited:
  - `cmd/ds/main.go`
  - `internal/commands/map.go`
  - `internal/commands/map_test.go`
  - `internal/commands/tldr.go`
  - `internal/commands/tldr_test.go`
  - `internal/commands/init.go`
  - `internal/commands/init_test.go`
  - `internal/commands/resume.go`
  - `internal/commands/v01_test.go`
  - `README.md`
  - `TASK_WORKFLOW_EXAMPLE.md`
- Tests run:
  - `go test ./internal/commands -run TestRecentCommandShowsRecentTopics|TestMapRecentFlagRemainsCompatibilityPath|TestMapRecentTextAvoidsTaskStatusClaims|TestTLDR_HumanOutputGroupsWorkflows|TestInitCreatesConfig|TestResume_EmptyRepoGuidance -count=1`
  - `go run ./cmd/ds --help`
  - `go run ./cmd/ds recent --no-refresh --max-areas 2`
  - `go run ./cmd/ds map --help`
