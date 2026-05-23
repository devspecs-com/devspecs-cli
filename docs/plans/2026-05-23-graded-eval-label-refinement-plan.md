# Graded Eval Label Refinement Plan

Date: 2026-05-23

## Goal

Make the real-repo eval steer on context-bundle usefulness instead of exact-file precision alone.

The eval should still preserve exact precision, recall, must-have recall, and sufficiency as guardrails, but graded precision should become visible in summaries and batch reports so useful supporting artifacts do not look identical to noise.

## Label Model

Use the existing `expected_relevant` importance field as the canonical label surface:

- `must`: required primary artifact, weight `1.0`
- `helpful`: directly useful supporting context, weight `0.6`
- `background`: weak but reasonable context, weight `0.3`
- `same_cluster`: acceptable sibling or line-range neighbor, weight `0.5`

Keep `expected_excluded` and `success_criteria.must_not_contain_artifacts` as hard negatives.

## Implementation

1. Promote graded precision into first-class eval summary fields:
   - mean graded precision
   - mean penalized utility precision
   - grade counts
2. Include graded precision in text reports, first-index reports, and the real50 runner aggregate.
3. Teach exact artifact metrics to treat `same_cluster` as a supported importance value without inflating must/helpful/background recall.
4. Refine real50 labels only where the current retrieved artifact is clearly supporting context:
   - add obvious supporting siblings as `helpful` or `background`
   - do not label artifacts just because the current retriever found them
   - keep hard negatives explicit
5. Rerun real50 control and treatment reports after label refinement.

## Auditable Success Criteria

- Unit tests cover summary-level graded precision.
- `ds eval` JSON summary includes `mean_graded_precision`, `mean_penalized_utility_precision`, and `grade_counts`.
- Text output prints graded precision next to exact precision.
- The real50 runner aggregate includes weighted graded precision fields.
- Label changes are visible in `cases.yaml` diffs and use only `must`, `helpful`, `background`, or `same_cluster`.
- Exact precision/recall remain available unchanged for comparison.

## Guardrails

- Do not remove hard negatives to improve metrics.
- Do not relabel broad generic docs as helpful without a query-specific reason.
- Do not promote graded precision as a replacement for must-have recall or sufficiency.
- Treat this as eval fidelity work, not a retrieval optimization.

## Implementation Results

Implemented on 2026-05-23:

- `ds eval` summary JSON now includes `mean_graded_precision`, `mean_penalized_utility_precision`, and aggregate `grade_counts`.
- Text reports and first-index batch reports print graded precision beside exact artifact precision.
- The real50 runner aggregate now emits weighted/failure-adjusted graded precision and penalized utility precision.
- `same_cluster` is accepted as an eval importance value for graded/ranking metrics without turning it into a must/helpful/background recall bucket.
- Conservative real50 label refinements were added only for query-specific supporting context in ai-shifu, apache/dolphinscheduler, apache/druid, and AReaL.

Validation:

- `go test ./internal/evalharness ./internal/commands -count=1`
- Dev tier, 12 repos / 31 cases / 0 failures:
  - exact precision `0.3124`
  - graded precision `0.3104`
  - penalized utility precision `0.2534`
  - recall `0.7581`
  - must-have recall `0.7742`
  - sufficiency `0.7742`
- Full control, 47 repos / 116 cases / 0 failures:
  - exact precision `0.3449`
  - graded precision `0.3481`
  - penalized utility precision `0.3163`
  - recall `0.7177`
  - must-have recall `0.7931`
  - sufficiency `0.7845`
  - must-hit@3 `0.6983`
- Full balanced-evidence treatment, 47 repos / 116 cases / 0 failures:
  - exact precision `0.3449`
  - graded precision `0.3481`
  - penalized utility precision `0.3163`
  - recall `0.7177`
  - must-have recall `0.7931`
  - sufficiency `0.7845`
  - must-hit@3 `0.8017`

Interpretation:

- Graded precision is a usefulness lens, not a guaranteed higher score than exact precision. Helpful/background labels are intentionally discounted, while exact precision still treats expected artifacts as binary hits.
- Balanced evidence remains a ranking improvement here: it changes first-hit ordering, not the selected artifact set.
