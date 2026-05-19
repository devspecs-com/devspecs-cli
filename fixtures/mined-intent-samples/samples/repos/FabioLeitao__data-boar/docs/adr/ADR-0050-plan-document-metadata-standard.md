# ADR 0050 — Plan document metadata standard

- **Status:** Accepted
- **Date (UTC):** 2026-05-12
- **Authors:** Fabio Leitao
- **Deciders:** Fabio Leitao

## Context

`PLAN_*.md` files are the primary vehicle for feature intent, sequencing, and implementation
context in this repository. As the plan corpus grows, inconsistent or absent document-level
metadata increases navigation cost: contributors and AI agents cannot determine — from the
file header alone — who owns a plan, when it was created, what its priority horizon is, or
which ADRs and other plans it depends on.

This is distinct from the MADR standard for ADRs (ADR-0045): plans are **living documents**
that evolve through slices and phases; they are not immutable architectural decisions with
a supersession lifecycle. Extending ADR-0045 to cover plans would conflate two different
document contracts.

The existing machine-readable comment convention (`<!-- plans-hub-summary: -->`,
`<!-- plans-hub-related: -->`) used by `scripts/plans_hub_sync.py` must be preserved.
This ADR adds a complementary structured header block above those comments.

## Decision

New `PLAN_*.md` files must open with the following header block, immediately after the `#` title:

```markdown
**Status:** Pending | Active | Completed | Deferred
**Date:** YYYY-MM-DD
**Authors:** name(s)
**Priority:** H0 | H1 | H2 | H3
**Depends on:** ADR-NNNN, PLAN_*.md (omit line if none)
```

Field semantics:

- **Status** — lifecycle aligned with `PLANS_TODO.md` states: `Pending` (backlog),
  `Active` (in-flight slices), `Completed` (all slices shipped, plan archived or archivable),
  `Deferred` (acknowledged, intentionally parked with a reason note).
- **Date** — creation date in UTC (ISO 8601). Update when the plan is substantially revised
  to reflect the last substantive edit, not every minor fix.
- **Authors** — person(s) responsible for the plan content and slice sequencing.
- **Priority** — horizon band from `PLANS_TODO.md` taxonomy: `H0` (critical path / now),
  `H1` (next sprint), `H2` (committed backlog), `H3` (future / research).
- **Depends on** — explicit links to ADRs and other plans that this plan builds upon or is
  constrained by. Omit the line entirely when there are no dependencies.

The existing `<!-- plans-hub-summary: -->` and `<!-- plans-hub-related: -->` comment
lines remain mandatory (governed by `scripts/plans_hub_sync.py`) and must follow the
header block.

**Backfill:** Not required for all existing plans immediately. Apply when a plan is
touched for a substantive update. New plans created after this ADR must comply at creation.
`scripts/add_plan_metadata.py` automates header injection for existing files and can be
reused as a scaffold reference when creating new plans.

## Rationale

1. **Agent and reviewer navigation**: a structured header answers "what is this plan's
   current status and priority?" without reading the full document body.
2. **Dependency traceability**: explicit `Depends on` links create a navigable plan graph,
   analogous to `Related Decisions` in ADRs, without duplicating ADR content.
3. **Separation of concerns**: plans and ADRs have different lifecycles. Plans evolve;
   ADRs are immutable decisions. A dedicated metadata standard for plans avoids the
   confusion of treating `PLAN_*.md` files as if they follow MADR semantics.
4. **Progressive adoption**: the backfill-on-touch rule avoids a big-bang migration
   that would only add noise to git history.

## Consequences

- **Positive:** New plans are immediately navigable by horizon, author, and dependency
  from the file header.
- **Positive:** `scripts/plans_hub_sync.py` and `scripts/plans-stats.py` can be
  progressively extended to extract and validate these fields.
- **Negative:** Slight additional overhead when creating a new plan (five header lines).
- **Ongoing:** When creating or substantially revising a `PLAN_*.md`, verify the header
  block is present and fields are current before committing.
- **Ongoing:** Every new `PLAN_*.md` — regardless of session keyword (`feature`, `docs`,
  `houseclean`, `backlog`) — must include the five-field header block at creation time.
  Use `scripts/add_plan_metadata.py` as a scaffold reference or run it against the new
  file immediately after creation to insert a correct placeholder block.
- **Ongoing:** `scripts/plans_hub_sync.py` — future enhancement: extract `Status`,
  `Priority`, and `Depends on` from the header block and surface them in `PLANS_HUB.md`.

## Alternatives Considered

1. **Extend ADR-0045 to cover plans** (rejected): ADRs and plans have different lifecycle
   semantics. A shared standard would either force plans into an inappropriate supersession
   model or weaken the ADR standard's immutability contract.
2. **YAML frontmatter (`---` block)** (rejected): breaks existing `plans_hub_sync.py`
   parsing of the first heading line; requires tooling changes before adoption is possible.
3. **No standard — free-form headers** (status quo, rejected): inconsistent metadata
   across ~30 plan files increases navigation cost and blocks future tooling queries.

## Related Decisions

- [ADR 0045 — ADR metadata and format standardization](ADR-0045-adr-metadata-and-format-standardization.md)
- [ADR 0048 — Operator-facing taxonomy and naming contract preservation](ADR-0048-operator-facing-taxonomy-and-naming-contract-preservation.md)

## References

- [`scripts/plans_hub_sync.py`](../../scripts/plans_hub_sync.py) — plan hub sync tooling
- [`scripts/plans-stats.py`](../../scripts/plans-stats.py) — plan status dashboard
- [`scripts/add_plan_metadata.py`](../../scripts/add_plan_metadata.py) — backfill and scaffold tool: injects the ADR-0050 header block into existing or new `PLAN_*.md` files
- [`docs/plans/PLANS_HUB.md`](../plans/PLANS_HUB.md) — auto-generated plan index
- [`docs/plans/PLANS_TODO.md`](../plans/PLANS_TODO.md) — horizon taxonomy (H0–H3)
