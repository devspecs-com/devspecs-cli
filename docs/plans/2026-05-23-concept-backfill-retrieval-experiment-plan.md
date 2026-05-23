# Concept Backfill Retrieval Experiment Plan

Date: 2026-05-23

## Context

The latest real50 run is close to the trust target:

- Sufficiency: 87.9%
- Must-have recall: 89.2%
- Query-baseline reduction: 97.3%
- Repo failures: 0/47

The remaining misses are mostly recall-specific and often have strong lexical/concept anchors:

- compact test identifiers, e.g. `testputandgetexposedtool`
- path/title concepts, e.g. `course visit analytics`, `fluxnova aigf`, `implementation plan`
- OpenSpec change/capability ids
- agent/skill/template filenames

A read-only spike against the older real50 control showed that deterministic concept scoring can rank many missed must-have artifacts above current selected negatives, especially for test/source and path-specific document queries. Some of that lift has already been captured by exact test-name anchors, so this experiment must be measured against the current post-anchor baseline.

## Hypothesis

A small, opt-in concept backfill lane can recover remaining missed must-have artifacts without materially increasing context noise:

- Generate deterministic concepts from query and candidate metadata.
- Score candidates that were not selected by normal retrieval.
- Add only 1-3 high-confidence candidates after primary retrieval.
- Preserve normal retrieval ranking and packing.
- Explain every backfilled artifact with matched compact identifiers, phrases, and path terms.

## Non-Goals

- Do not replace weighted retrieval or balanced evidence.
- Do not introduce embeddings, LLM scoring, or external services.
- Do not build a general semantic graph.
- Do not make this default until it beats the current baseline on real50.

## Implementation

### Retrieval

- Add `ConceptBackfill` to `WeightedFilesRetrieverV0`.
- Add retriever name suffix when enabled.
- After primary selection, balanced evidence, and OpenSpec expansion, append a small number of concept backfill candidates before section packing so recovered files still get section-trimmed.
- Candidate pool is the full corpus minus already selected artifacts.
- Backfill limits:
  - default: 1 candidate
  - strong identifier queries: 2 candidates
  - test/source behavior queries without a strong identifier: 1 candidate
  - non-specific queries: 0 candidates unless score is very high
- Score concepts using:
  - compact identifiers from query and candidate text
  - camel/Pascal/snake/kebab splitting
  - bigrams/trigrams from meaningful query terms
  - path/title/heading terms
  - test metadata (`test_name`, `parent_title`, symbols, assertion terms)
  - section headings and selected section metadata
- Store metadata on backfilled candidates:
  - `concept_backfill_score`
  - `concept_backfill_matched_compacts`
  - `concept_backfill_matched_phrases`
  - `concept_backfill_matched_path_terms`

### Eval Diagnostics

- Add per-case diagnostics for missed must-have artifacts:
  - `missed_must_concept_diagnostics`
  - `expected_path`
  - `in_candidate_pool`
  - `concept_rank`
  - `concept_score`
  - `matched_compacts`
  - `matched_phrases`
  - `matched_path_terms`
- Rank missed must-haves against the deterministic `query_file_baseline` candidate pool.
- Keep these diagnostics active even when concept backfill is disabled, so we can identify future recall opportunities.

### CLI / Runner

- Add eval flag `--experimental-concept-backfill`.
- Add runner flag `-ExperimentalConceptBackfill`.
- Keep the feature opt-in and excluded from default product behavior.

## Auditable Success Criteria

- Unit tests prove concept backfill can recover:
  - a compact test identifier not selected by ordinary retrieval
  - a specific product/requirements doc with strong path/title concepts
- Unit tests prove weak broad queries do not backfill noisy markdown.
- Eval JSON includes missed must-have concept diagnostics.
- Real50 fresh-cache run completes with 47/47 repos and 0 failures.
- Compared with the current post-anchor baseline:
  - must-have recall improves or stays flat
  - sufficiency improves or stays flat
  - precision does not regress by more than 1 percentage point
  - query-baseline reduction remains in the 90-97% band, or only slightly above 97% if trust metrics improve

## Measurement

Baseline:

`test-anchor-discovery-highsignal-freshcache-full-balanced-20260523`

Treatment:

Run full real50 with:

- `--include-tests`
- `--include-code-comments`
- `--experimental-balanced-evidence`
- `--experimental-concept-backfill`
- fresh eval index cache

Compare:

- weighted artifact precision
- weighted graded precision
- weighted artifact recall
- weighted must-have recall
- weighted context sufficiency
- weighted query-baseline reduction
- miss-class summaries
- missed must concept diagnostics

If treatment crosses 90% must-have recall without material precision loss, promote the experiment for further validation on a held-out set. Otherwise keep the diagnostics and do not promote the backfill lane.

## Experiment Result

Final treatment run:

`concept-backfill-tight-freshcache-full-balanced-20260523`

- Repo coverage: 47/47, 0 failures
- Precision: 38.98% -> 36.11%
- Graded precision: 41.63% -> 39.17%
- Recall: 80.27% -> 82.57%
- Must-have recall: 89.22% -> 91.81%
- Context sufficiency: 87.93% -> 90.52%
- Query-baseline reduction: 97.34% -> 96.75%

Conclusion: concept backfill is directionally useful for trust metrics and crosses the near-term recall/sufficiency targets, but it misses the precision-regression guard. Keep it opt-in while the next experiment focuses on reducing unlabeled backfills or making backfilled items tiered/overflow instead of always part of the main pack.
