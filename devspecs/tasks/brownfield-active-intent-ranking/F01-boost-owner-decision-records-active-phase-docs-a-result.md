# Task brownfield-active-intent-ranking F01 Result

## Summary
- Target: `F01` - Boost owner decision records active phase docs and Status next plans above blocked or superseded epics
- Outcome:

## Changed Files
-

## Tests
-

## Decision
-

## Follow-up
-

## References
- `F00-index.md`
- `F01-boost-owner-decision-records-active-phase-docs-a-plan.md`

## Checkpoints
- Use `ds task checkpoint brownfield-active-intent-ranking --target F01` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T11:22:41Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-112241-validated.md`
- Structured Evidence: `checkpoints/20260616-112241-validated.json`
- What changed: Added active intent authority signals so owner decision records, north-star active phase docs, and Status: next plans outrank blocked or superseded historical intent; scout demotion now moves blocked historical intent out of the working set when current intent is present.
- Evidence for decision: 5 file(s) edited; 2 test command(s)
- What remains: next target F02; next decision continue
- Next iteration: F02 with decision continue
- Files edited:
  - `internal/retrieval/retrieval.go`
  - `internal/retrieval/pack.go`
  - `internal/retrieval/pack_negative_evidence.go`
  - `internal/retrieval/retrieval_test.go`
  - `internal/retrieval/pack_negative_evidence_test.go`
- Tests run:
  - `go test ./internal/retrieval -count=1`
  - `go test ./internal/commands -run TestApplyFindPackScout|TestFindPack|TestWriteFindPack|TestDisplayPack -count=1`
