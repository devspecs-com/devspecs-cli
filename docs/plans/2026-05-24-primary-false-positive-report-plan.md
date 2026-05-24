# Primary False-Positive Report Plan

Date: 2026-05-24

## Goal

Make primary-pack precision failures auditable from the eval output itself.

The current real50 reports explain missed expected artifacts reasonably well, but primary false positives are mostly visible only as raw `irrelevant_included` lists and aggregate grade counts. That makes it too hard to tell whether low strict precision means bad retrieval, sparse labels, same-cluster useful context, or missing eval structure.

## Scope

Add diagnostics to `ds eval` JSON and text output:

- primary false-positive summaries grouped by query type, lane, diagnostic role, reason class, and grade
- per-case false-positive examples with position, path, lane, role, grade, reason class, and reason snippets
- extension coverage summaries for expected, retrieved, missed, missing-from-corpus, and primary false-positive artifacts
- unindexed document-format summaries for repo files that look like docs but were not indexed
- explicit diagnostic role for AsciiDoc paths such as `.adoc`, `.asciidoc`, and `.asc`

## Non-Goals

- Do not change retrieval ranking in this pass.
- Do not change precision semantics.
- Do not auto-forgive unlabeled or same-cluster artifacts.
- Do not add a UI.

## Measurement Implications

The report should help separate:

- true primary-pack noise that ranking should demote
- useful-but-unlabeled context where labels should be expanded
- same-cluster artifacts where graded precision is the better utility measure
- missing corpus support, including document formats like AsciiDoc
- eval expectation gaps, where one query expects too narrow or too broad a set

## Auditable Success Criteria

- `ds eval --json` includes `diagnostics.false_positive_summaries`.
- Each false-positive summary includes count, grade counts, query type, lane, role, reason class, and examples.
- Each case includes `primary_false_positive_diagnostics` for non-exact primary artifacts.
- `ds eval --json` includes `diagnostics.extension_summaries`.
- `ds eval --json` includes `diagnostics.unindexed_document_summaries`.
- `.adoc`, `.asciidoc`, and `.asc` expected paths are classified as `asciidoc` document gaps in diagnostics.
- Repo-local `.adoc` files that are not part of the indexed corpus are visible as unindexed document-format gaps.
- Existing headline metrics remain unchanged.
- Unit tests assert false-positive and extension diagnostics are populated.
- A real50 smoke/full run can be inspected for top false-positive classes without ad hoc scripts.

## Decision Rule

Use this report to select the next narrow ranking patch only when a repeated false-positive class appears across multiple repos or cases. If the top classes are mostly unlabeled same-cluster artifacts, refine eval labels before tuning retrieval.
