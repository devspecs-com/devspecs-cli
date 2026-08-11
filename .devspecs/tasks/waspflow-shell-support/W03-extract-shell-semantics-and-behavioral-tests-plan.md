# Task waspflow-shell-support W03 Plan

## Goal
Extract shell semantics and behavioral tests.

## Problem
Whole-file shell artifacts improve recall but do not identify the functions, sourced modules, command dispatch, provider contracts, or behavioral checks that make a large shell program understandable.

## Expected Change Surface
- `internal/adapters/sourcecontext/sourcecontext.go`
- `internal/adapters/testcase/testcase.go`
- source-manifest and relationship extraction
- focused shell fixtures and adapter tests

## Work
- Evaluate a maintained shell parser before choosing bounded parsing rules; document the dependency and failure tradeoff.
- Extract named shell functions, `source`/`.` imports, and top-level command dispatch anchors without treating variable expansion as a symbol.
- Recognize shell behavior tests in dedicated test files and verification scripts.
- Distinguish real behavioral cases from helper functions and fake provider implementations in monolithic scripts such as `scripts/verify.sh`.
- Emit source/test relationships that retrieval can use without duplicating entire large files into every context pack.
- Fail loss-safely: an unparseable shell file remains available as whole-file source context.

## Acceptance
- `bin/waspflow` exposes command handlers and dispatch anchors.
- `lib/providers/*.sh` exposes provider adapter functions and sourced-module relationships.
- `scripts/verify.sh` yields named behavioral cases while helper/fake functions are not mislabeled as separate tests.
- Extracted line ranges are valid, bounded, deterministic, and traceable to source.
- Large shell files remain within indexing memory and time budgets.
- Existing language extraction fixtures remain unchanged unless an explicit improvement is reviewed.

## Test Standards
- Use testify and one test per behavioral case.
- Use Prepare, Act, Assert once per test.
- Assert collection length before individual elements.
- Prefer checked-in minimal shell fixtures over large mocked parser outputs.

## Decision Gates
- Promote: extracted semantics materially improve query evidence with low false-positive noise.
- Improve: source symbols work but test-case boundaries need a separate refinement.
- Rework: regex or parser behavior cannot provide stable line-level evidence.
- Rollback: extraction replaces loss-safe whole-file context or causes unbounded scan cost.
