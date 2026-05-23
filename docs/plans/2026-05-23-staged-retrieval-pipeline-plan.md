# Staged Retrieval Pipeline Plan

Date: 2026-05-23

Depends on:

- `2026-05-23-local-glossary-v0-experiment-plan.md`
- `2026-05-23-tiered-concept-backfill-plan.md`

## Goal

Make retrieval phases explicit and measurable so future precision/recall experiments do not become intertwined scoring tweaks.

Target shape:

`base candidates -> strong anchors -> relationship expansion -> concept/glossary backfill -> role-diverse packing -> diagnostics`

## Current Problem

The current retriever still mostly scores every candidate, trims, expands OpenSpec links, optionally backfills, and then packs sections. That works, but it makes failures hard to diagnose:

- Was a must-have missing from discovery?
- Was it in the candidate pool but below threshold?
- Was it backfilled but displaced?
- Was it included only as a broad same-cluster artifact?
- Did section packing remove the useful part?

Staging should make these answers visible and testable.

## Non-Goals

- Do not rewrite all retrieval scoring in one pass.
- Do not require embeddings or LLM routing.
- Do not change default behavior until staged and non-staged evals can be compared.
- Do not use git history in this plan.

## Proposed Stages

### 1. Base Candidate Scoring

Current weighted candidate scoring.

Outputs:

- ranked scored candidates
- score reasons
- role/lane
- authority cues

### 2. Strong Anchors

Promote candidates with precise signals:

- exact test-name compact
- exact path/title phrase
- OpenSpec change/capability id
- ADR/RFC/PRD title match
- explicit requested file/path

Outputs:

- anchor type
- anchor confidence
- matched query terms

### 3. Relationship Expansion

Expand from anchors:

- OpenSpec parent/child/companion links
- sibling/neighbor sections
- markdown links
- same artifact parent sections
- test/source around exact identifier anchors

Outputs:

- expansion reason
- source anchor
- relationship type

### 4. Concept/Glossary Backfill

Use glossary-supported concept matches for recall recovery.

Outputs:

- concept score
- glossary support
- matched concepts
- admission tier

### 5. Role-Diverse Packing

Pack with budgets per query intent:

- intent docs
- tests/source evidence
- protocol/template/model docs only when requested
- related/overflow tier

Outputs:

- pack tier
- lane budget usage
- dropped candidates and reasons

### 6. Diagnostics

Emit per-case and per-query diagnostics:

- candidate-pool presence
- missed must-have stage
- selected stage
- concept rank
- anchor rank
- pack tier
- section pack ranges

## Auditable Success Criteria

- Existing retrieval tests pass with staged mode disabled.
- Experimental staged mode produces equivalent or better real50 trust metrics than current concept/glossary mode.
- Eval JSON can group selected artifacts by stage and pack tier.
- Miss diagnostics identify the first failed stage for every missed must-have:
  - missing from corpus
  - not scored
  - scored below threshold
  - backfill rejected
  - relationship not expanded
  - packed out
- Real50 run completes 47/47 repos with zero failures.
- Staged mode does not reduce query-baseline token reduction below 90%.

## Measurement

Compare:

- current best baseline
- concept backfill
- glossary-gated concept backfill
- staged retrieval

Primary metrics:

- must-have recall
- context sufficiency
- graded precision
- strict precision
- low-precision sufficient cases
- first failed stage for misses
- token reduction

Decision:

Promote staged retrieval only if it improves explainability and does not regress north-star trust metrics.
