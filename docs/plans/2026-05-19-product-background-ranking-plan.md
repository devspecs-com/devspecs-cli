# Product Background Ranking Plan

Date: 2026-05-19

## Context

The retrieval precision pass raised the indexed eval to 78.5% mean precision, but the remaining weak lane is product/background retrieval. The current product-background query keeps the must-have PRD, but it still admits adjacent product docs and plans that share broad words such as billing, customer, portal, access, and entitlement.

The product/background lane should behave differently from implementation and source-file lanes:

- Prefer the PRD or requirements artifact whose path/title matches the named subject.
- Include accepted ADRs when they provide durable background decisions, such as source-of-truth or boundary decisions.
- Avoid adjacent PRDs, plans, source files, and OpenSpec deltas that only share broad body vocabulary.

## Goals

- Improve precision for product/background queries without regressing discovery coverage.
- Keep the scoring logic general: subject anchoring, artifact role, accepted decision authority, and off-topic path/title evidence.
- Add tests that prove adjacent PRDs/plans do not win merely because their bodies mention common product terms.

## Non-Goals

- Do not add ML or external dependencies.
- Do not tune against individual filenames.
- Do not relabel genuinely irrelevant adjacent product docs as relevant just to improve metrics.

## Implementation Plan

1. Add a product-subject profile.
   - Use the existing core query terms as subject terms.
   - Count subject matches in path/title separately from body-only matches.
   - Detect adjacent product surfaces in path/title when they are not requested by the query.

2. Tighten product/background scoring.
   - Strongly boost PRDs whose path/title matches multiple subject terms.
   - Strongly penalize same-family PRDs with weak path/title subject agreement.
   - Strongly demote plans, agent notes, source files, OpenSpec artifacts, and RFCs unless explicitly requested.
   - Let accepted ADRs through when they match multiple subject terms and contain decision-authority cues such as source-of-truth or boundary language.

3. Add regression tests.
   - Product background should select the named PRD and durable ADRs.
   - Adjacent PRDs, portal notes, admin override decisions, and source files should not be selected for the product-background query.

## Acceptance Criteria

- `prd-background-entitlements` precision improves materially from 20.0%.
- Overall mean precision does not regress from 78.5%.
- Mean must-have recall remains at or above the current 93.2%.
- Discovery coverage remains 100%.
- `go test -count=1 ./...` passes.

## Implemented Result

Final local indexed eval after this pass:

- `prd-background-entitlements` precision: 100.0% (from 20.0%)
- `prd-background-entitlements` recall: 100.0%
- `prd-background-entitlements` tokens: 677
- Overall mean artifact precision: 85.8% (from 78.5%)
- Overall mean must-have recall: 93.2%
- Overall mean artifact recall: 83.9%
- Mean token reduction vs full planning corpus: 94.0%
- Context sufficiency: 10/11
- Discovery coverage: 100.0%
- Expected missing from indexed corpus: 0

The remaining non-sufficient case is `resume-entitlement-sync`, which still misses the ADR and agent handoff/plan context. That should be handled as a resume-context bundle retrieval pass, not as product/background scoring.
