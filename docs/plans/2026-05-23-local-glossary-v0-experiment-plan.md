# Local Glossary V0 Experiment Plan

Date: 2026-05-23

## Goal

Recover concept-backfill precision without giving up the recall and sufficiency gains from deterministic concept matching.

Current measured state:

- Baseline `test-anchor-discovery-highsignal-freshcache-full-balanced-20260523`
  - precision: 38.98%
  - recall: 80.27%
  - must-have recall: 89.22%
  - sufficiency: 87.93%
  - query-baseline reduction: 97.34%
- Concept backfill `concept-backfill-tight-freshcache-full-balanced-20260523`
  - precision: 36.11%
  - recall: 82.57%
  - must-have recall: 91.81%
  - sufficiency: 90.52%
  - query-baseline reduction: 96.75%

The backfill lane is useful, but it admits too many unlabeled artifacts. Local Glossary V0 should act as a precision control layer between query concepts and backfill admission.

## Hypothesis

Concept backfill should only promote candidates through concepts that are locally evidenced by durable artifact fields:

- path segments
- titles
- headings
- test names and parent test titles
- classifier/OpenSpec metadata
- selected section headings

Broad repo terms and generic documentation words should not act as backfill keys, even if they appear in the query or candidate body.

## Non-Goals

- No LLM or embeddings.
- No global synonym list.
- No persistent schema migration in V0.
- No manual label dependency.
- No use of glossary as a hard filter for normal retrieval.
- No promotion to default until eval results clear precision and trust guardrails.

## Implementation

Add an opt-in retrieval flag:

`--experimental-glossary-concepts`

Behavior:

1. Build an in-memory glossary from already indexed candidates for each retrieval call.
2. Extract glossary concepts from stable local evidence:
   - filename and directory phrases
   - artifact title
   - section headings
   - indexed section match headings
   - test names and parent titles
   - OpenSpec roles/change/capability metadata
3. Track per-concept evidence:
   - document frequency
   - evidence kinds
   - example paths
   - canonical label
4. Resolve query concepts against the glossary:
   - compact identifiers, e.g. `testputandgetexposedtool`, `fluxnova`
   - local bigrams/trigrams, e.g. `course visit analytics`
   - path/title terms only when they are part of a glossary-supported phrase or compact
5. Gate concept backfill:
   - backfill candidates must match at least one glossary-supported concept
   - exact test-name compacts remain eligible when locally evidenced by tests
   - broad concepts are suppressed by document-frequency thresholds
6. Add explainable metadata:
   - `concept_glossary_enabled`
   - `concept_glossary_matched_json`
   - `concept_glossary_evidence_json`

## Auditable Success Criteria

- Unit tests show a repo-wide term such as a project name is rejected as a backfill concept when it appears across many paths.
- Unit tests show a rare local product/spec concept can still backfill a relevant PRD/spec document.
- Unit tests show an exact compact test-name concept remains eligible.
- Eval flag is separate from `--experimental-concept-backfill` so the impact is isolated.
- Real50 run completes 47/47 repos with zero failures.
- Compared with concept backfill:
  - precision improves by at least 1 percentage point, or unlabeled backfilled artifacts drop by at least 20%
  - must-have recall does not regress by more than 1 percentage point
  - sufficiency does not regress by more than 1 percentage point
  - query-baseline reduction remains between 90% and 97.5%
- Compared with baseline:
  - must-have recall and sufficiency remain improved
  - precision loss is smaller than concept backfill alone

## Measurement

Run full real50 with:

- `--include-tests`
- `--include-code-comments`
- `--experimental-balanced-evidence`
- `--experimental-concept-backfill`
- `--experimental-glossary-concepts`
- fresh eval index cache

Compare against:

- baseline: `test-anchor-discovery-highsignal-freshcache-full-balanced-20260523`
- concept backfill: `concept-backfill-tight-freshcache-full-balanced-20260523`

Primary metrics:

- weighted artifact precision
- weighted graded precision
- weighted artifact recall
- weighted must-have recall
- weighted context sufficiency
- low-precision sufficient cases
- total unlabeled artifacts
- total hard negatives
- query-baseline token reduction

Decision:

- If glossary recovers precision while preserving most of the recall/sufficiency lift, keep it as the preferred concept-backfill mode for validation.
- If it loses the recall lift, keep only diagnostics and proceed to tiered output.

## Experiment Result

Treatment run:

`glossary-concept-backfill-freshcache-full-balanced-20260523`

Compared with baseline:

- Precision: 38.98% -> 37.41%
- Graded precision: 41.63% -> 40.31%
- Recall: 80.27% -> 82.57%
- Must-have recall: 89.22% -> 91.81%
- Context sufficiency: 87.93% -> 90.52%
- Query-baseline reduction: 97.34% -> 96.89%

Compared with concept backfill alone:

- Precision: 36.11% -> 37.41% (+1.30 pp)
- Graded precision: 39.17% -> 40.31% (+1.14 pp)
- Recall: 82.57% -> 82.57% (flat)
- Must-have recall: 91.81% -> 91.81% (flat)
- Context sufficiency: 90.52% -> 90.52% (flat)
- Total unlabeled artifacts: 254 -> 246
- Backfilled artifacts in selected packs: 45 -> 35
- Backfilled unlabeled artifacts: 29 -> 21

Conclusion: Local Glossary V0 is directionally useful and preserved the concept-backfill trust lift while recovering part of the precision loss. It still does not return precision to baseline, so it should remain experimental. The next precision lever should be tiered concept output, because several remaining strict-precision "misses" are same-cluster or related evidence that should likely be visible outside the primary pack.
