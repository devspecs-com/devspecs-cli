---
status: proposed
tags: [retrieval, intent-graph, indexing, heuristics, multi-stack]
---

# Language-Agnostic Intent Graph

Change ID: `language-agnostic-intent-graph`

## Summary

Introduce a language-agnostic artifact graph as the next retrieval architecture direction for DevSpecs. The goal is to improve retrieval quality without overfitting to TypeScript, React, Go, or any single stack.

DevSpecs should treat source symbols, markdown sections, OpenSpec changes, ADRs, PRDs, plans, and agent notes as evidence feeding a shared graph:

```text
artifact -> section -> entity -> edge -> retrieval packet
```

AST extraction may become a useful evidence source, but it should not become the core abstraction or a requirement for useful retrieval.

## Motivation

The current seed eval shows strong compression but weak precision and insufficient context in many cases. Some proposed fixes mention TypeScript source symbols because the fixture is TypeScript/React + API shaped, but DevSpecs must work across stack and repo boundaries.

The May 12 trial report and the Kleio CLI inspection point toward a broader deterministic approach:

- normalize local entities
- preserve identifier-shaped terms
- extract section boundaries from plans/docs
- learn aliases from local evidence
- build explainable artifact relationships
- rank with entity overlap, authority, lifecycle, section role, and graph edges

This can improve retrieval without LLM calls, embeddings, or network dependencies.

## Goals

- Define a language-neutral artifact graph model.
- Make section and entity extraction useful before AST extraction.
- Represent source symbols generically as `source_symbol` or `code_entity`.
- Add artifact-type adapters for OpenSpec, ADR, PRD, plans, and agent notes.
- Add deterministic edges for companion artifacts, lifecycle, path references, aliases, and constraints.
- Keep retrieval reasons auditable in eval JSON.
- Add eval coverage that proves the approach is not TypeScript-specific.

## Non-goals

- Do not implement LLM judging.
- Do not require embeddings or external model calls.
- Do not introduce a graph database dependency for v0.
- Do not require a supported AST parser for useful retrieval.
- Do not make TypeScript or React concepts first-class retrieval primitives.
- Do not claim broad semantic understanding beyond deterministic local evidence.

## Success Criteria

- DevSpecs has a documented language-neutral retrieval architecture.
- Eval cases can verify source/context retrieval without relying only on TypeScript.
- Retrieval output can explain graph/entity/section reasons for included artifacts.
- AST or language-specific symbol extraction is optional and additive.
- Before/after eval result files show movement in must-have recall, precision, and sufficiency without collapsing token reduction.
