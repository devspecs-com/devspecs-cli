# Duplicate And Variant Collapse Plan

Date: 2026-05-22

Order: 1

Depends on:

- [0] Authority priors

Enables:

- [2] Section indexing + packing
- [3] Tiered output

## Goal

Reduce context noise by grouping duplicate or near-duplicate artifact variants before final bundle packing.

This should improve precision without hiding plausible alternates from the agent.

## Variant Families

Collapse should be conservative and explainable.

Likely variant families:

- Translation mirrors, such as `docs/en/...` and `docs/zh/...`.
- Archive/current copies.
- Example/project/template copies of the same artifact shape.
- Generated/reference dump copies.
- Root instruction files versus nested module instruction files.
- OpenSpec bundle child siblings where parent/child structure already explains relation.
- Same title or slug across adjacent directories.

## Design

Add a `VariantGroup` step after candidate scoring and before final packing.

Each candidate may receive:

- `variant_group_id`
- `variant_role`, such as canonical, translation, archive, example, template, generated, nested-module, sibling
- `variant_reason`
- `canonical_candidate_id`

Selection rules:

- Keep the strongest canonical/current/default-language candidate in primary tier.
- Keep exact query path/title matches even if they look like variants.
- Keep expected/eval-labeled variants visible during eval if they are the target.
- Move collapsed siblings to nearby candidates with reasons instead of deleting them.
- Do not collapse unrelated documents just because they share generic names like `README.md` or `design.md`.

## Eval Integration Lift

Low to medium.

File-level eval can continue unchanged if collapsed siblings remain surfaced in metadata. Add optional diagnostics:

- duplicate pressure before/after
- collapsed sibling count
- whether an expected artifact was selected, nearby, or hidden

No new ground-truth labels are required for the first pass.

## Auditable Success Criteria

- Real dev50 precision improves without lowering must-have recall below the current optimized baseline.
- Canonical fixture must-have recall does not regress.
- Collapsed candidates remain inspectable as nearby/collapsed candidates with reasons.
- Unit tests cover translation mirror selection.
- Unit tests cover archive/current selection.
- Unit tests cover template/instance selection.
- Unit tests cover root `CLAUDE.md` versus nested module `CLAUDE.md`.
- Unit tests prove exact query subject/path matches are not collapsed away.

## Rollback Criteria

- Expected artifacts disappear from all surfaced tiers.
- Collapse groups merge unrelated docs with generic filenames.
- Variant grouping becomes repo-specific or depends on mined sample names.
