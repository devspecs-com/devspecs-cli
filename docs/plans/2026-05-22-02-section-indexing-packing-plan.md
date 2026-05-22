# Section Indexing And Packing Plan

Date: 2026-05-22

Order: 2

Depends on:

- [0] Authority priors
- [1] Duplicate / variant collapse

Enables:

- [3] Tiered output
- [4] Query intent routing / lane budgets

## Goal

Move retrieval output from whole-file packing toward artifact-aware section packing.

The system should still retrieve files/artifacts reliably, but it should pack only the sections most useful for the agent when a file is large or partially relevant.

## Why This Matters

File-level recall can be good while the final bundle remains noisy.

Section-level packing can improve:

- token savings
- precision of the actual agent context
- trust, because included sections can cite heading paths and line ranges
- debuggability, because nearby sections can be surfaced without full bodies

## Design

Add section units for markdown artifacts.

Each section should track:

- parent artifact id
- source path
- heading path
- heading depth
- line range or byte range
- normalized title tokens
- body summary or excerpt
- extracted tasks/checklists
- extracted acceptance criteria
- explicit markdown links
- frontmatter inheritance

Initial retrieval shape:

1. Score bundle/collection when present.
2. Score file/artifact.
3. Score sections inside selected files and strong nearby files.
4. Pack selected sections into primary context.
5. Keep full file path/id for expansion.

Do not require section labels for the first implementation. Preserve file-level eval as the primary guardrail.

## Section References

Start with simple parent-child references:

- section belongs to file
- file belongs to bundle/collection when applicable
- section may link to sibling/other sections through markdown links

Do not build a full cross-section graph initially. Record links so future graph features can use them.

## Eval Integration Lift

Medium, but manageable if staged.

Stage 1:

- Keep existing file-level labels and metrics.
- Add pack-level metrics:
  - packed token count
  - selected section count
  - expected artifact represented in packed context
  - full-file fallback count

Stage 2:

- Add optional section labels only for selected development cases.
- Measure section hit rate and section precision.

This should not derail progress if file-level recall remains the release gate.

## Auditable Success Criteria

- File-level must-have recall does not regress on canonical or real dev50 evals.
- Packed context token count decreases versus whole-file packing for large artifacts.
- Every packed section includes source path and heading path.
- If a file is short, whole-file packing remains allowed.
- If no section scores clearly, fallback behavior preserves the current file-level context.
- Unit tests cover heading extraction, nested headings, task extraction, frontmatter inheritance, and code-block handling.
- Eval output records whether a returned artifact was represented by full file or selected sections.

## Rollback Criteria

- Section packing hides must-have artifact content needed for context sufficiency.
- Section extraction becomes brittle on common markdown.
- The eval becomes dependent on section labels before enough labels exist.
