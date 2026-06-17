# Task brownfield-active-intent-ranking F03 Result

## Summary
- Target: `F03` - Add fixtures for brownfield recovery before artifacts exist and after current decision docs exist
- Outcome: Added deterministic brownfield recovery fixtures for the ScopeLab-style failure mode. The fixtures preserve historical/blocked docs as visible evidence before current artifacts exist, then require owner decision and active-phase docs to become the retrieval anchors once they exist. This exposed and fixed a real gap where `north_star` active-phase docs could be filtered as weak body-only backfill while stale exact-word historical plans remained selected.

## Changed Files
- `internal/retrieval/retrieval.go`
- `internal/retrieval/retrieval_test.go`
- `internal/retrieval/pack_negative_evidence_test.go`

## Tests
- `go test ./internal/retrieval -run "TestWeightedFilesRetrieverV0_BrownfieldRecoveryBeforeAndAfterCurrentDecisionDocs|TestWeightedFilesRetrieverV0_RanksActiveIntentAboveBlockedHistoricalPlan|TestWeightedFilesRetrieverV0_ExactPlanID|TestApplyDemotionOnlyNegativeEvidence" -count=1`
- `go test ./internal/retrieval -count=1`
- `go test ./internal/commands -run "TestApplyFindPackScout|TestFindPack|TestWriteFindPack|TestDisplayPack" -count=1`

## Decision
- Promote. The brownfield recovery lifecycle now has explicit regression coverage and current-intent ranking semantics are stronger: active decision/active-phase artifacts are anchored when present, inactive historical intent is demoted in non-history queries, and active-phase body evidence is not filtered as weak generic documentation.

## Follow-up
- Keep the product stance: `ds find` routes to canonical current docs; it does not invent owner decisions before artifacts exist.
- If F continues, focus on command-level fixture coverage for real indexed artifacts rather than more unit-only scoring tests.

## References
- `F00-index.md`
- `F03-add-fixtures-for-brownfield-recovery-before-arti-plan.md`

## Checkpoints
- Use `ds task checkpoint brownfield-active-intent-ranking --target F03` to append structured evidence.

### Checkpoint
- Created At: 2026-06-17T13:36:06Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260617-133606-validated.md`
- Structured Evidence: `checkpoints/20260617-133606-validated.json`
- What changed: Added brownfield recovery fixtures before/after current decision docs and fixed current-intent dominance so active decision docs beat blocked or superseded historical intent.
- Evidence for decision: 4 file(s) read; 4 file(s) edited; 3 test command(s)
- What remains: next target command-level-brownfield-fixture; next decision improve
- Next iteration: command-level-brownfield-fixture with decision improve
- Files read:
  - `devspecs/tasks/brownfield-active-intent-ranking/F00-index.md`
  - `devspecs/tasks/brownfield-active-intent-ranking/F01-boost-owner-decision-records-active-phase-docs-a-result.md`
  - `devspecs/tasks/brownfield-active-intent-ranking/F02-tighten-exact-plan-id-and-track-id-find-packs-so-result.md`
  - `internal/retrieval/retrieval.go`
- Files edited:
  - `internal/retrieval/retrieval.go`
  - `internal/retrieval/retrieval_test.go`
  - `internal/retrieval/pack_negative_evidence_test.go`
  - `devspecs/tasks/brownfield-active-intent-ranking/F03-add-fixtures-for-brownfield-recovery-before-arti-result.md`
- Tests run:
  - `go test ./internal/retrieval -run 'TestWeightedFilesRetrieverV0_BrownfieldRecoveryBeforeAndAfterCurrentDecisionDocs|TestWeightedFilesRetrieverV0_RanksActiveIntentAboveBlockedHistoricalPlan|TestWeightedFilesRetrieverV0_ExactPlanID|TestApplyDemotionOnlyNegativeEvidence' -count=1`
  - `go test ./internal/retrieval -count=1`
  - `go test ./internal/commands -run 'TestApplyFindPackScout|TestFindPack|TestWriteFindPack|TestDisplayPack' -count=1`
