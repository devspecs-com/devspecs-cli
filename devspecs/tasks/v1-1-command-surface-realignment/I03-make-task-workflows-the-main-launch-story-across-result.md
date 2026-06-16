# Task v1-1-command-surface-realignment I03 Result

## Summary
- Target: `I03` - Make task workflows the main launch story across tldr help README and docs
- Outcome: promoted task-first launch copy across root help, task help, `ds tldr`, README, and the public workflow transcript. `ds map`, `ds recent`, and `ds find` are now framed as diagnostic/evidence/trust layers rather than required setup before known work.

## Changed Files
- `cmd/ds/main.go`
- `cmd/ds/main_test.go`
- `internal/commands/task.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`

## Tests
- `go test ./cmd/ds ./internal/commands -run TestRootCmd_HelpCentersTaskWorkflow|TestRootCmd_HelpMentionsTelemetryPrivacy|TestRootCmd_TLDRRegistered|TestRootCmd_PublicHelpHidesInternalCommands|TestTLDR_HumanOutputGroupsWorkflows|TestTLDR_FilterAndJSON|TestTLDR_UnknownWorkflowErrorsWithValidIDs -count=1`
- `go run ./cmd/ds --help`
- `go run ./cmd/ds tldr`
- `go run ./cmd/ds task --help`

## Decision
- Promote to `I04`.

## Follow-up
- `I04`: harden rich task result completion contract for promote/improve iteration loops.

## References
- `I00-index.md`
- `I03-make-task-workflows-the-main-launch-story-across-plan.md`

## Checkpoints
- Use `ds task checkpoint v1-1-command-surface-realignment --target I03` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T09:23:41Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-092341-validated.md`
- Structured Evidence: `checkpoints/20260616-092341-validated.json`
- Files read:
  - `internal/commands/tldr.go`
  - `README.md`
- Files edited:
  - `cmd/ds/main.go`
  - `cmd/ds/main_test.go`
  - `internal/commands/task.go`
  - `internal/commands/tldr.go`
  - `internal/commands/tldr_test.go`
  - `README.md`
  - `TASK_WORKFLOW_EXAMPLE.md`
- Tests run:
  - `go test ./cmd/ds ./internal/commands -run TestRootCmd_HelpCentersTaskWorkflow|TestRootCmd_HelpMentionsTelemetryPrivacy|TestRootCmd_TLDRRegistered|TestRootCmd_PublicHelpHidesInternalCommands|TestTLDR_HumanOutputGroupsWorkflows|TestTLDR_FilterAndJSON|TestTLDR_UnknownWorkflowErrorsWithValidIDs -count=1`
  - `go run ./cmd/ds --help`
  - `go run ./cmd/ds tldr`
  - `go run ./cmd/ds task --help`

### Checkpoint
- Created At: 2026-06-16T13:17:30Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-131730-validated.md`
- Structured Evidence: `checkpoints/20260616-131730-validated.json`
- What changed: Refined the launch-story placement so brownfield workflows show ds task immediately after init when work is actionable, with map/recent/find/context positioned as optional trust checks around the task instead of a mandatory preflight.
- Evidence for decision: 3 file(s) edited; 1 test command(s)
- What remains: -
- Next iteration: promote to the next slice
- Files edited:
  - `internal/commands/tldr.go`
  - `internal/commands/tldr_test.go`
  - `README.md`
- Tests run:
  - `go test ./cmd/ds ./internal/commands -run TestRootCmd_TLDRRegistered|TestTLDR_HumanOutputGroupsWorkflows|TestTLDR_FilterAndJSON|TestTLDR_UnknownWorkflowErrorsWithValidIDs -count=1`
