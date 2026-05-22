# Lane-Aware Retrieval Precision Plan

## Context

The subtype-first classifier correctly marks `agent_instruction` and `skill` as protocol subtypes, but the first retrieval experiment showed a ranking regression. Demoting protocol artifacts removed `AGENTS.md` noise, then unrelated body-only markdown backfilled the result budget.

The fix should improve retrieval selection quality without adding repo-specific rules or hiding discovery/classification issues.

## Goal

Keep non-intent lane demotion, but make retrieval budget filling evidence-aware:

- Strong candidates should satisfy path/title, identifier, role, or multiple core-query evidence.
- Weak body-only candidates should not backfill a result set when stronger candidates already cover the query.
- Protocol/model/template artifacts should remain retrievable when the query explicitly asks for those modes or subtypes.

## Implementation

1. Add a scored-candidate quality tier after scoring.
2. Treat candidates with path/title core matches, identifier matches, explicit source matches, or role/query alignment as strong.
3. Treat broad markdown candidates with only generic body matches as weak when the query has multiple core terms.
4. Select strong candidates up to the normal retrieval limit first.
5. Drop weak body-only candidates when anchored evidence exists; if no anchored evidence exists, preserve the existing fallback behavior.
6. Add tests for the observed regression pattern using generic broad docs rather than repo-specific path exceptions.
7. Rerun unit tests and the fast labeled retrieval dev comparison before any broader validation pass.

## Success Criteria

- `agent_instruction` and `skill` remain protocol subtypes; reporting should not count them as separate lanes.
- Ordinary intent queries do not retrieve protocol instructions unless explicitly requested.
- A query with a clear subject term does not backfill unrelated broad markdown that only matches generic body words.
- Existing source, RFC, product-background, OpenSpec, lifecycle, and protocol-request tests pass.
- Labeled fast retrieval comparison does not regress sufficiency versus the previous discovery-purity baseline.

## Non-Goals

- Do not add per-repository file exceptions.
- Do not change artifact discovery coverage.
- Do not broaden the lane taxonomy in this pass.
