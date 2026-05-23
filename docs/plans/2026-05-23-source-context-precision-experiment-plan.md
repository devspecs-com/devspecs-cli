# Source Context Precision Experiment Plan

Date: 2026-05-23

## Goal

Improve real50 retrieval precision without sacrificing the useful test-case recall gain.

The current precision failures are mostly source-context noise:

- raw test files are returned alongside more precise test-case units
- sibling test variants are returned when one unit-level test is enough
- Java and Rust tests are not parsed as units, so retrieval falls back to broad docs or unrelated raw test files
- non-test queries can still pull raw test files through path/body term matches

## Experiment

Implement a conservative source-context precision pass:

1. Add JVM and Rust test-unit extraction to the existing `intent:test_case` adapter.
2. Prefer line-scoped test-case artifacts over raw whole-file test candidates.
3. Suppress raw test source files for non-test queries unless the query explicitly asks for source files.
4. Keep raw test files as fallback when no unit-level test candidate exists.
5. Keep code-comment artifacts gated separately; do not broaden comment retrieval in this pass.

## Non-Goals

- No repo-specific query terms or path names.
- No LLM reranking.
- No full code index.
- No semantic drift detection.
- No hidden eval-label-specific rules.

## Auditable Success Criteria

- Unit tests prove Java/JUnit-style test methods are extracted with line ranges.
- Unit tests prove Rust `#[test] fn ...` units are extracted with line ranges.
- Unit tests prove a line-scoped test case suppresses its raw whole-file test candidate.
- Unit tests prove non-test planning queries do not retrieve raw test files.
- Real50 focused smoke improves precision on affected source-context cases without lowering must-have recall for those cases.
- Canonical Go tests pass.
- Any real50 report names the fixed run directory and baseline being compared.

## Rollback Criteria

- Must-have recall drops on the fixed real50 source-context cases.
- Raw test file suppression hides the only available relevant artifact when no test-case unit exists.
- Rules become repository-specific or depend on the current mined fixture names.

