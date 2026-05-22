# Test Case Intent Artifacts Plan

Date: 2026-05-22

Depends on:

- Section indexing + packing
- Conservative retrieval variant collapse

## Goal

Treat test cases as searchable behavioral intent artifacts without building a general code index or drift detector.

Tests can encode expected behavior, edge cases, regressions, API contracts, and domain vocabulary. V0 should make that signal available to retrieval when the query asks for behavior, bugs, regressions, APIs, validation, permissions, billing, analytics, retries, or similar implementation-facing context.

## Scope

Add an experimental `test_case` adapter that discovers likely test files and emits individual test-case artifacts.

Supported v0 test units:

- Go: `func TestXxx`
- Python: `def test_*`
- JS/TS: `describe`, `context`, `it`, `test`
- Ruby/RSpec: `describe`, `context`, `it`
- PHP/PHPUnit: `function test*`, methods with `#[Test]`

Each artifact should include:

- source path
- line range
- inferred language/framework
- test title/name
- parent describe/context title when cheaply available
- bounded body snippet
- lightweight symbol and assertion vocabulary

## Symbol Policy

Extract symbols and assertion vocabulary only as weak retrieval features.

Do not infer implementation facts, drift, or correctness from tests in v0. The metadata may say "this test mentions `stripe_event_id` and `assertEquals`," but not "the implementation guarantees idempotency."

## Toggle And Eval

The feature must be independently measurable:

- repo config experiment: `test_case_artifacts`
- scan flag: `--experimental-test-cases`
- eval flag: `--experimental-test-cases`

Default behavior remains unchanged unless the experiment is enabled.

## Retrieval Behavior

Test artifacts are supporting context.

- Boost them for behavior/regression/API/edge-case/test queries.
- Do not include them for ordinary planning/ADR/PRD queries unless lexical evidence is strong.
- Avoid flooding packs with many tests.
- Preserve source provenance and line ranges in packed context.

## Auditable Success Criteria

- `ds scan --experimental-test-cases` indexes individual test cases with `kind=source_context`, `subtype=test_case`.
- `ds find` can retrieve test cases by test title, symbol, or assertion vocabulary.
- Retrieved test-case context includes source path and line range.
- Eval can run with and without `--experimental-test-cases`.
- Real-repo eval reports precision, recall, must-have recall, context sufficiency, token count, and test-case artifact count.
- Existing eval without the flag is unchanged.
- Unit tests cover Go, Python, JS/TS, Ruby, and PHP extraction heuristics.

## Rollback Criteria

- Test artifacts noticeably flood default retrieval.
- Must-have recall or context sufficiency regresses on the fixed real-dev eval.
- Extraction creates unstable source identities across repeated scans.
- The implementation drifts into broad source-code indexing or drift claims.
