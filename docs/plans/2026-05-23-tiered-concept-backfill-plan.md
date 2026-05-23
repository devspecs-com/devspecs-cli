# Tiered Concept Backfill Plan

Date: 2026-05-23

Depends on:

- `2026-05-23-concept-backfill-retrieval-experiment-plan.md`
- `2026-05-23-local-glossary-v0-experiment-plan.md`

## Goal

Preserve concept backfill's recall benefits while reducing main-pack precision pressure by separating high-confidence context from plausible related context.

The current eval treats every selected artifact as part of the main pack. That is harsh for backfill candidates that are useful as "related evidence" but not necessary enough to spend primary context budget.

## Hypothesis

A tiered output model can improve user trust and measured utility:

- primary pack: strong anchors and high-confidence backfills
- related/overflow: plausible concept matches, sibling tests, historical notes, and low-confidence relationship expansions
- omitted diagnostics: candidates considered but not included

This should let DevSpecs be permissive without hiding too much or polluting the main context.

## Non-Goals

- Do not remove candidate discovery or retrieval recall diagnostics.
- Do not hide concept matches from users or agents.
- Do not change eval labels to forgive noisy context without measuring it.
- Do not require UI work in this pass.

## Implementation Sketch

1. Add pack tier metadata:
   - `pack_tier=primary`
   - `pack_tier=related`
   - `pack_tier=diagnostic`
2. Add tier assignment after retrieval:
   - exact must-like anchors stay primary
   - relationship-expanded OpenSpec companions stay primary when role-requested
   - glossary-supported rare concept backfills may stay primary
   - broad concept matches become related
   - same-cluster test/source matches become related unless exact test-name requested
3. Update `ds find` and `resume-query` JSON to expose tiers.
4. Update eval:
   - strict precision remains based on primary pack
   - related artifacts get separate utility accounting
   - sufficiency may count related only in an explicit tier-aware eval mode
5. Add diagnostics:
   - primary token count
   - related token count
   - related relevant count
   - related hard-negative count
   - must-have present in primary vs related

## Auditable Success Criteria

- A focused unit test proves exact test-name matches land in primary while sibling tests land in related.
- A focused unit test proves same-concept but unlabeled backfill can be demoted to related.
- Eval output includes primary and related artifact lists separately.
- Real50 tier-aware eval completes 47/47 repos with zero failures.
- Primary precision improves over concept backfill.
- Must-have recall in primary does not regress materially from the current best run.
- Related tier shows useful recall coverage without being counted as hidden success.

## Measurement

Compare:

- baseline
- concept backfill
- glossary-gated concept backfill
- tiered concept backfill

Primary metrics:

- primary precision
- primary must-have recall
- primary sufficiency
- related recall lift
- related hard-negative rate
- token split: primary vs related

Decision:

Promote tiering only if it gives a cleaner main context while still making potentially useful evidence inspectable.

## 2026-05-23 Experiment Result

Implemented as an opt-in eval/runtime experiment:

- `--experimental-tiered-concept-output`
- `pack_tier=primary|related`
- strict eval metrics remain primary-pack only
- related tier is emitted separately with precision/graded-precision diagnostics

Full real50 run:

`tiered-glossary-concept-backfill-freshcache-full-balanced-20260523`

Compared with `glossary-concept-backfill-freshcache-full-balanced-20260523`:

- precision: `0.3741 -> 0.3767`
- graded precision: `0.4031 -> 0.4059`
- recall: unchanged at `0.8257`
- must-have recall: unchanged at `0.9181`
- sufficiency: unchanged at `0.9052`
- low-precision sufficient cases: `75 -> 74`
- related tier: `4` artifacts, `0` exact relevant, `1` same-cluster helpful signal

Readout: tiering is safe and slightly cleaner, but it is not a major precision lever yet. Most noise is still coming from primary retrieval/ranking and label coverage, not from lower-confidence concept backfill overflow.
