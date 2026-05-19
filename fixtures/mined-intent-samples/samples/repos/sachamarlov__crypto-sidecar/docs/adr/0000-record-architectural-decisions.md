# 0000 — Record architectural decisions using MADR

- Status: accepted
- Date: 2026-04-20
- Deciders: @sachamarlov, Claude Opus 4.7
- Tags: [process, documentation]

## Context and problem statement

GuardiaBox is built end-to-end by a single human plus an AI agent in a tight
academic timebox, and is intended to live on as a portfolio asset. Without a
disciplined record of _why_ each non-trivial choice was made, future-us (or
the academic evaluator) will rediscover trade-offs from scratch and risk
overturning decisions that were defensible the first time around.

## Considered options

- **Option A — MADR (Markdown Architectural Decision Records)** v4, one file
  per decision, sequentially numbered.
- **Option B — Free-form notes** in a single `DECISIONS.md` file appended over
  time.
- **Option C — No formal records**, rely on commit messages and PR
  descriptions.

## Decision

Adopt **MADR v4** under `docs/adr/`. Each non-trivial architectural or
cryptographic decision gets its own file with the canonical sections
(_Context_, _Considered options_, _Decision_, _Consequences_).

## Consequences

**Positive**

- Decisions are searchable, immutable, and survive contributor turnover.
- Each decision links to its corresponding spec or code.
- The format is well-known to AI agents, lowering the cost of automating
  decision-record generation.

**Negative**

- Adds a small per-decision overhead (~10 minutes of writing).
- Discipline must be enforced in code review.

## References

- MADR project — https://adr.github.io/madr/
- "When to write an ADR" — https://github.com/joelparkerhenderson/architecture-decision-record
