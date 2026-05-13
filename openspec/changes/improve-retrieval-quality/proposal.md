---
status: proposed
tags: [eval, retrieval, context, quality]
---

# Improve Retrieval Quality

Change ID: `improve-retrieval-quality`

## Summary

Improve DevSpecs retrieval quality while preserving substantial context-token reduction. The seed eval now shows strong compression but mediocre recall and weak precision:

- Mean token reduction vs full planning corpus: ~80.3%
- Mean artifact recall: ~68.8%
- Mean artifact precision: ~36.6%

This change proposes deterministic retrieval and eval improvements before any LLM-based judging. The work should make DevSpecs better at preparing compact, relevant agent context for repo intent artifacts.

## Motivation

The May 12 trial report exposed concrete retrieval gaps:

- Identifier-shaped terms such as `client_display`, `pnpm`, and `authorization_details`.
- Dated and slug-style plan filenames.
- Missing OpenSpec `design.md` and spec deltas from context.
- Lifecycle ambiguity around stale, superseded, ready-to-archive, and unknown-status artifacts.
- Results that lack snippets, paths, line numbers, and selection reasons.
- Reliance on short IDs instead of stable slugs and filenames.
- Missed likely plan directories outside defaults.

The current seed eval now exposes similar gaps in a reproducible fixture. The next goal is not perfect retrieval; it is measurable improvement on recall, must-have recall, precision, and sufficiency while keeping token reduction high.

## Goals

- Add importance-weighted eval relevance: `must`, `helpful`, `background`.
- Add deterministic context sufficiency checks.
- Improve identifier-aware matching for path, body, title, task, and source content.
- Retrieve OpenSpec proposal/design/tasks/spec deltas as coherent bundles when appropriate.
- Add lifecycle and authority signals to reduce stale/noisy artifact inclusion.
- Add query intent classification for implementation, product background, stale history, source identifiers, and resume workflows.
- Add score reasons for explainable retrieval.

## Non-goals

- Do not add LLM judging yet.
- Do not optimize only for token reduction.
- Do not tune weights to make visible seed cases perfect.
- Do not claim agent coding success from this eval.
- Do not replace existing `ds scan`, `ds find`, or `ds context` UX wholesale in this change.

## Success Criteria

- Eval output reports overall recall, must-have recall, precision, sufficiency pass rate, and token reduction.
- Seed eval cases can mark expected artifacts by importance.
- Retrieval changes are justified by trial-report failures, not fixture-specific hacks.
- OpenSpec implementation context includes design/spec deltas when the query asks for agent implementation context.
- Identifier-shaped queries find relevant source and planning artifacts.

