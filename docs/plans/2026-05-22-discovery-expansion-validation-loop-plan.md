# Discovery Expansion Validation Loop

## Goal

Promote the proposal/RFC/roadmap/architecture discovery expansion only if it improves held-out first-index behavior without creating obvious precision debt.

## Sequence

1. Commit the current discovery expansion as a rollback checkpoint.
2. Build `ds` from that checkpoint.
3. Run the existing validation-50 set, or a practical validation slice if the full set is too slow.
4. Compare validation diagnostics against the previous clean/baseline reports where available.
5. Diagnose remaining `expected_missing_from_corpus` and `missed_after_discovery` paths.
6. Apply one narrow, general patch only if diagnostics point to a repeatable feature-family gap.
7. Rerun the same validation slice after the patch.

## Patch Discipline

- Prefer general feature families over repo names.
- Do not add broad `docs/**` indexing.
- Do not tune ranking to hide discovery failures.
- Keep the evaluation slice fixed before and after any narrow patch.
- Revert the narrow patch if validation recall/coverage does not improve or if precision drops materially.

## Success Criteria

- Validation discovery coverage improves for proposal/RFC/roadmap/architecture cases.
- `expected_missing_from_corpus` decreases on the fixed validation slice.
- Must-have recall does not regress.
- Artifact precision does not materially regress.
- Any remaining gaps are classified as discovery, corpus-scope, ranking, or label issues.
