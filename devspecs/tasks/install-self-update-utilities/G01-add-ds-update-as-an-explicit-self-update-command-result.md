# Task install-self-update-utilities G01 Result

## Summary
- Target: `G01` - Add ds update as an explicit self-update command with package-manager-aware guidance
- Outcome: implemented a guidance-only `ds update` command with package-manager-aware update instructions and JSON output.

## Changed Files
- `cmd/ds/main.go`
- `internal/commands/update.go`
- `internal/commands/update_test.go`

## Tests
- `go test ./internal/commands -run "TestDetectInstallSource|TestUpdateReport|TestOutputUpdateReport" -count=1`
- `go test ./cmd/ds -count=1`
- `go test ./internal/commands -run "TestDetectInstallSource|TestUpdateReport|TestOutputUpdateReport|TestDOD_01_Install" -count=1`
- `go build -ldflags "...version metadata..." -o .devspecs/bin/ds.exe ./cmd/ds`
- `.devspecs/bin/ds.exe update`
- `.devspecs/bin/ds.exe update --json`
- `.devspecs/bin/ds.exe --help`

## Decision
- Promote to `G02`.

## Follow-up
- `G02`: add lightweight version staleness detection without checking on every command.
- Consider an apply mode later only after install-source detection is proven across real installs.

## References
- `G00-index.md`
- `G01-add-ds-update-as-an-explicit-self-update-command-plan.md`

## Checkpoints
- Use `ds task checkpoint install-self-update-utilities --target G01` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T13:50:58Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-135058-validated.md`
- Structured Evidence: `checkpoints/20260616-135058-validated.json`
- What changed: Implemented guidance-only ds update with install-source detection for Homebrew, Scoop, Go install, local development builds, and manual installs. Added text and JSON output plus focused detection/output tests.
- Evidence for decision: 4 file(s) read; 4 file(s) edited; 7 test command(s)
- What remains: next target G02; next decision promote
- Next iteration: G02 with decision promote
- Files read:
  - `devspecs/tasks/install-self-update-utilities/G01-add-ds-update-as-an-explicit-self-update-command-plan.md`
  - `internal/commands/version.go`
  - `cmd/ds/main.go`
  - `README.md`
- Files edited:
  - `cmd/ds/main.go`
  - `internal/commands/update.go`
  - `internal/commands/update_test.go`
  - `devspecs/tasks/install-self-update-utilities/G01-add-ds-update-as-an-explicit-self-update-command-result.md`
- Tests read:
  - `internal/commands/update_test.go`
- Tests run:
  - `go test ./internal/commands -run TestDetectInstallSource|TestUpdateReport|TestOutputUpdateReport -count=1`
  - `go test ./cmd/ds -count=1`
  - `go test ./internal/commands -run TestDetectInstallSource|TestUpdateReport|TestOutputUpdateReport|TestDOD_01_Install -count=1`
  - `go build -ldflags ... -o .devspecs/bin/ds.exe ./cmd/ds`
  - `.devspecs/bin/ds.exe update`
  - `.devspecs/bin/ds.exe update --json`
  - `.devspecs/bin/ds.exe --help`
