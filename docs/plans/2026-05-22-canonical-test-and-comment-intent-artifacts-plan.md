# Canonical Test And Comment Intent Artifacts Plan

Date: 2026-05-22

## Goal

Promote executable tests from an experiment to a canonical optional `intent:test_case` artifact source, then add a cautious optional `intent:code_comment` source for high-signal implementation comments.

## Scope

1. Rename user-facing test controls away from `experimental` while keeping backward-compatible aliases.
2. Add config-level artifact switches:
   - `artifacts.test_cases`
   - `artifacts.code_comments`
3. Keep both sources opt-in for now because large repositories can contain thousands of units.
4. Tighten retrieval so test/comment artifacts only enter packs when the query asks for behavior, regressions, implementation rationale, invariants, or code-level context.
5. Add a code-comment adapter that extracts only intent-bearing comments, not license headers or ordinary narration.
6. Extend eval plumbing so tests and comments can be measured independently from core markdown intent docs.

## Non-Goals

- No full code index.
- No AST graph.
- No drift detection.
- No claim that comments are authoritative. They are supporting intent evidence.
- No default-on test/comment indexing until performance and pack-noise risks are measured.

## Retrieval Policy

Tests are behavioral intent. Comments are implementation-local rationale or constraints.

Both should be excluded from ordinary ADR/PRD/RFC/roadmap context unless the query clearly asks for:

- tests, coverage, regression, edge case, validation, behavior
- source/code implementation context
- rationale, invariant, workaround, assumption, constraint, compatibility, security
- identifiers that strongly anchor to the source artifact

Default pack budgets should be small:

- test cases: 2-4 supporting artifacts unless explicitly test-focused
- code comments: 1-3 supporting artifacts unless explicitly comment/rationale-focused

## Auditable Success Criteria

- `ds scan --include-tests` indexes test cases without using experimental terminology.
- `ds scan --include-code-comments` indexes high-signal comments as `mode=intent`, `subtype=code_comment`.
- `ds eval --include-tests` and `ds eval --include-code-comments` work for indexed evals.
- Existing `--experimental-test-cases` remains accepted as a deprecated alias.
- Existing test-case evals still pass.
- New unit tests verify:
  - test cases are not selected for ordinary roadmap/product queries
  - test cases are selected for behavior/regression queries
  - code comments are selected for rationale/invariant queries
  - license/header comments are ignored
- `go test ./...` passes.

## Follow-Up Measurement

After implementation, rerun the real50 common-set comparison:

- baseline
- `--include-tests`
- `--include-tests --include-code-comments`

Compare recall, must-have recall, sufficiency, precision, retrieved test/comment counts, and timeout pressure.
