# Task scanless-workflow-ux A01 Result

## Summary
- Target: `A01` - Fold ds scan into workflow commands so onboarding starts with map, find, and task instead of a required manual scan
- Outcome: Validated and promoted. Workflow commands now refresh by default in the public UX, while `ds scan` is framed as manual refresh/rebuild.

## Changed Files
- `README.md`
- `internal/commands/init.go`
- `internal/commands/init_test.go`
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- `internal/commands/resume.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `internal/commands/v01_test.go`

## Tests
- `go test ./internal/commands -run TestInit_CreatesRepoConfig|TestTLDR|TestMapAutoScanLeavesUsableIndexForFindPack|TestMapNoRefreshSkipsAutoScan|TestMapJSONAutoScanKeepsStdoutJSON|TestResume_EmptyRepo`
- `go test -timeout 20m ./internal/commands`
- `go test -timeout 20m ./...`

## Decision
- Promote.

## Follow-up
- Consider a future `ds pack <query>` alias if human friction around `find --pack` persists.

## References
- `A00-index.md`
- `A01-fold-ds-scan-into-workflow-commands-so-onboardin-plan.md`

## Checkpoints
- Use `ds task checkpoint scanless-workflow-ux --target A01` to append structured evidence.

### Checkpoint
- Created At: 2026-06-10T13:52:10Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260610-135210-validated.md`
- Structured Evidence: `checkpoints/20260610-135210-validated.json`
- Files read:
  - `internal/commands/init.go`
  - `internal/commands/map.go`
  - `internal/commands/tldr.go`
  - `internal/commands/resume.go`
- Files edited:
  - `README.md`
  - `internal/commands/init.go`
  - `internal/commands/init_test.go`
  - `internal/commands/map.go`
  - `internal/commands/map_test.go`
  - `internal/commands/resume.go`
  - `internal/commands/tldr.go`
  - `internal/commands/tldr_test.go`
  - `internal/commands/v01_test.go`
- Tests run:
  - `go test ./internal/commands -run TestInit_CreatesRepoConfig|TestTLDR|TestMapAutoScanLeavesUsableIndexForFindPack|TestMapNoRefreshSkipsAutoScan|TestMapJSONAutoScanKeepsStdoutJSON|TestResume_EmptyRepo`
  - `go test -timeout 20m ./internal/commands`
  - `go test -timeout 20m ./...`
