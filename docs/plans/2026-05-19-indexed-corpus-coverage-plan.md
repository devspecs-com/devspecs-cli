---
title: Indexed Corpus Coverage Improvement Plan
kind: plan
status: draft
tags: [eval, indexing, coverage, openspec, source-context]
---

# Indexed Corpus Coverage Improvement Plan

## Purpose

Improve the existing `ds eval` first-index scoreboard by fixing indexed corpus coverage gaps before tuning retrieval weights or classifier heuristics.

The current indexed eval diagnostics show that weak first-index performance is primarily a coverage problem:

```text
Mean token reduction: 73.3%
Mean artifact recall: 40.7%
Mean must-have recall: 46.7%
Mean artifact precision: 17.0%
Context sufficiency: 2/10
Discovery coverage: 45.7%
Retrieval coverage of discovered expected artifacts: 68.8%
```

The key missing expected artifact families are:

- OpenSpec companion files: `design.md`, `tasks.md`, and `specs/*/spec.md`
- source/context files: TypeScript and SQL files referenced by implementation queries
- agent continuation notes: `.claude/notes/**`

## Scope

This pass should improve what the first index can see. It should not broaden retrieval heuristics enough to hide precision problems.

In scope:

- Preserve OpenSpec proposal/design/tasks/spec delta files as retrievable indexed candidates.
- Preserve OpenSpec layout groups so companion files remain linkable as one change bundle.
- Index bounded source/context candidates that are useful for AI-agent implementation context.
- Include `.claude/notes/**` in markdown discovery.
- Keep `ds eval` as the measurement path and keep diagnostics in the existing eval JSON/text output.

Out of scope:

- New eval framework or new public command.
- API_SPEC vs OpenSpec classifier tuning.
- ADR/PRD/RFC false-positive tuning.
- Plan retrieval precision tuning.
- Broad all-file indexing without size/path/type guardrails.

## Design

### OpenSpec Companion Indexing

Today the OpenSpec adapter indexes the `proposal.md` file as the primary artifact and reads `tasks.md` only to add todos. That hides `design.md`, `tasks.md`, and spec deltas from retrieval even when eval cases require those exact paths.

Change discovery to emit one OpenSpec candidate per known child file:

```text
openspec/changes/<id>/proposal.md
openspec/changes/<id>/design.md
openspec/changes/<id>/tasks.md
openspec/changes/<id>/specs/<capability>/spec.md
```

Each child artifact should:

- use the child path as the source path
- share the change directory as `layout_group`
- use `format_profile: openspec`
- keep subtype `openspec_change`
- produce a title that includes the change id plus child role when no H1 exists

The proposal artifact may continue to aggregate task todos from `tasks.md` for resume workflows, but the tasks file must also be retrievable as its own source.

### Source Context Candidate Indexing

The eval expected source files are currently absent from the SQLite-indexed corpus. Add a conservative source-context adapter that indexes only bounded text/code candidates:

```text
*.ts, *.tsx, *.js, *.jsx, *.sql
```

Guardrails:

- respect `.gitignore`, `.git/info/exclude`, and `.aiignore`
- skip generated/vendor/build directories through the existing ignore matcher and adapter-local skips
- skip markdown files
- skip very large files
- index source files as `kind: source_context`
- use source path as source identity

This makes live `ds find` and `ds resume <query>` operate on the same indexed corpus that `ds eval` measures.

### Agent Notes

Add `.claude/notes` to default markdown source coverage so continuation notes can be indexed by the existing markdown adapter.

## Audit Commands

Run:

```bash
go test ./internal/evalharness ./internal/commands ./internal/adapters/openspec ./internal/adapters/sourcecontext ./internal/config
go run ./cmd/ds eval ./fixtures/agentic-saas-fragmented --json --no-save
```

The JSON eval output should show:

- `diagnostics.discovery_coverage` materially above the current `0.457`
- OpenSpec `design`, `tasks`, and spec delta paths absent from `diagnostics.expected_missing_from_corpus`
- source files absent from `diagnostics.expected_missing_from_corpus`
- `.claude/notes/webhook-idempotency-followup.md` absent from `diagnostics.expected_missing_from_corpus`
- `corpus.source_context_candidates.files > 0`

## Success Criteria

- Focused tests pass locally.
- Pre-commit `go test ./...` passes if a commit is created.
- Indexed eval discovery coverage improves from `45.7%` to at least `85%`.
- The implementation does not add a new eval command or new eval fixture requirement.
- The source-context adapter uses bounded path/type/size rules rather than indexing arbitrary repo files.
- Retrieval precision changes are recorded but not optimized in this pass.
