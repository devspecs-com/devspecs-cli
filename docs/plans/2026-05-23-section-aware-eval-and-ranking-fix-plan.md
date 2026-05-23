# Section-Aware Eval And Ranking Fix Plan

Date: 2026-05-23

## Goal

Make the canonical eval measure the same section-aware retrieval behavior users get from `ds find` / `ds resume`, then tune section ranking so indexed sections improve context quality instead of flooding retrieval with broad matches.

## Problem

The current default indexed eval scans fixtures into SQLite, reads artifact rows into memory, and calls the retriever directly. That path persisted sections but did not exercise the query-aware section retrieval bridge used by live commands.

Result:

- default indexed eval is mostly a regression guard for artifact retrieval and section packing
- live `find` eval exercises section FTS, but section hits are too broad and reduce precision/sufficiency
- we cannot trust section-aware impact numbers until eval and production share the same retrieval path

## Scope

Implement now:

- shared indexed candidate loading used by commands and eval
- section rows attached to retrieval candidates as first-class evidence
- default indexed eval exercises section-aware retrieval
- eval ablation flag to disable section-aware retrieval
- conservative section ranking and budgets
- JSON/text diagnostics for section-selected artifacts and hits

Keep out of scope:

- master-data/entity index
- embeddings or LLM reranking
- full section-level manual labels
- generalized code symbol graph

## Design

### Shared Candidate Loading

Create a shared internal package for indexed retrieval candidate loading.

Both command and eval paths should use the same logic for:

- artifact body rendering
- classifier metadata projection
- OpenSpec link metadata
- persisted markdown section attachment
- query-aware section match metadata when a query is available

The command package should keep thin wrappers only if tests or command code still need legacy helper names.

### Retrieval Candidate Sections

Extend `retrieval.Candidate` with persisted section evidence:

- section id
- source path
- heading path
- line range
- title
- body/excerpt
- token estimate
- section kind
- inherited metadata

The retriever should compute section matches from `Candidate.Sections` so default eval, live commands, and cached indexed eval use the same ranking logic.

### Eval Modes

Default indexed eval:

- uses shared indexed candidate loading
- includes persisted sections
- runs section-aware retrieval by default

Ablation:

- `--disable-section-aware-retrieval`
- disables section boosts and selected-section metadata while preserving artifact/file retrieval
- allows baseline vs section-aware comparison without changing the binary

Live command eval:

- remains useful as a smoke for Cobra/JSON/output path
- should no longer be the only path that exercises section-aware retrieval

### Section Ranking

Section retrieval is additive but conservative.

Rules:

- require at least one non-generic query/core/identifier hit in heading/body for section-selected artifact admission
- prefer heading and identifier matches over body-only generic matches
- cap selected sections per artifact
- cap section-selected artifact boost so it cannot swamp stronger file-level evidence
- avoid section rescue for protocol/model/template artifacts unless the query asks for that mode
- avoid archive/stale/superseded boosts unless lifecycle intent is present
- do not let generic headings such as `Overview`, `Introduction`, or `Background` select an artifact without body-specific evidence

## Auditable Success Criteria

- Default `ds eval --indexed --json` reports non-zero `section_selected_*` metrics on a fixture with matching persisted sections.
- `ds eval --indexed --disable-section-aware-retrieval --json` reports zero section-selected artifacts while keeping section packing available for selected files.
- `ds eval --command find --json` and default indexed eval use the same shared indexed candidate construction path.
- Section-selected artifacts include heading path and line range in reasons or metadata.
- Eval cache key changes when section-aware candidate shape changes.
- Existing command JSON remains backward-compatible; new section metadata is additive.
- Unit tests cover:
  - shared indexed candidate loader adds persisted sections
  - default eval path can produce section-selected artifacts
  - ablation disables section-selected artifacts
  - section ranking rejects generic body-only matches
  - identifier/heading section hits can rescue a parent artifact
- Focused real-dev50 eval produces a baseline and section-aware comparison in one binary.
- Section-aware run must not regress must-have recall by more than 2 percentage points versus ablation.
- Section-aware run should improve precision, graded precision, or context sufficiency versus ablation before promotion.
- Live command path should not select materially more artifacts than default indexed eval for the same fixture/query without an explainable command-output reason.

## Rollback Criteria

- Section-aware retrieval lowers must-have recall by more than 2 percentage points.
- Section-aware retrieval improves recall only by adding broad false positives.
- Section hit reasons lack heading/range provenance.
- Eval runtime grows enough that the focused dev tier no longer completes in a short feedback loop.

## Implementation Outcome

The shared indexed candidate path and ablation flag are implemented. Full real50 measurement showed that section evidence was useful as diagnostics, but not yet trustworthy as an artifact-admission or ranking boost:

- section-ranking enabled initially reduced full real50 precision, recall, must-have recall, and sufficiency versus ablation
- making section evidence ranking-neutral restored artifact metrics to the ablation baseline
- indexed section metadata remains attached to selected artifacts for audit/reasons
- existing full-body section packing remains the context-packing path until indexed section packing can prove no sufficiency regression

Current conservative contract:

- persisted sections are queryable and attached to retrieval candidates
- default indexed eval exercises section-aware enrichment and reports non-zero `section_selected_*`
- `--disable-section-aware-retrieval` disables the enrichment for ablation
- section evidence does not admit or reorder artifacts yet
- roadmap section evidence is gated behind roadmap/release/future-planning intent

Promotion of section evidence into ranking should require a future experiment that beats ablation on precision, graded precision, or sufficiency without reducing must-have recall.
