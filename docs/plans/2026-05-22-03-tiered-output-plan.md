# Tiered Output Plan

Date: 2026-05-22

Order: 3

Depends on:

- [0] Authority priors
- [1] Duplicate / variant collapse
- [2] Section indexing + packing

Enables:

- [4] Query intent routing / lane budgets

## Goal

Replace binary include/exclude retrieval output with an auditable tiered context bundle.

The bundle should reduce default token load without hiding uncertainty from the agent.

## Output Tiers

Suggested tiers:

- Primary: high-confidence artifacts/sections packed into the main context body.
- Supporting: relevant but secondary artifacts/sections, possibly shorter excerpts.
- Nearby: plausible candidates shown as metadata only.
- Collapsed variants: duplicate/variant siblings not packed.
- Excluded near misses: candidates intentionally not packed, with reasons.

## Design

Extend focused context output with structured tiers.

Example shape:

```markdown
# DevSpecs Focused Context

Query: ...
Confidence: medium-high

## Primary Context

...

## Supporting Context

...

## Nearby Candidates

- docs/product-specs/teacher-analytics-dashboard.md
  Reason: analytics match, different product surface

## Collapsed Variants

- docs/zh/architecture/design.md
  Reason: translation mirror of docs/en/architecture/design.md
```

JSON output should expose the same tiers as arrays with reasons, confidence, and token estimates.

## Hiding Policy

Do not fully hide plausible candidates in early versions.

- Main context can be strict.
- Nearby/collapsed/excluded metadata should remain visible.
- Full body expansion should be possible with `ds context <id>`.

## Eval Integration Lift

Medium.

Existing file-level metrics continue to apply to all surfaced tiers or primary tier, depending on report mode.

Add two metric views:

- Primary precision/recall: measures what the agent receives by default.
- Surfaced recall: measures primary + supporting + nearby metadata visibility.

This prevents precision improvements from silently hiding important artifacts.

## Auditable Success Criteria

- Primary context token count is lower or equal to current whole-file output.
- Surfaced must-have recall stays at or above current optimized baseline.
- Primary precision improves on real dev50.
- Every non-primary candidate has an explicit reason and tier.
- JSON and markdown outputs include the same tier information.
- Unit tests cover primary/supporting/nearby/collapsed/excluded tier assignment.
- Existing `ds resume --json` consumers can still access a flat artifact list during transition.

## Rollback Criteria

- Agents lose visibility into plausible must-have candidates.
- Tiering makes output harder to consume programmatically.
- Primary precision improves only by moving many must-have artifacts to nearby metadata.
