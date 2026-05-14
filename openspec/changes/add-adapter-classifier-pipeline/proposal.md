---
status: proposed
tags: [adapters, classification, indexing, retrieval]
---

# Add Adapter Classifier Pipeline

Change ID: `add-adapter-classifier-pipeline`

## Summary

Introduce a deterministic adapter classifier pipeline so DevSpecs can identify intent artifacts by document features, not only by configured paths.

The intended pipeline is:

```text
broad safe candidate discovery
-> universal feature extraction
-> container and document classifiers score candidates
-> resolver chooses model or generic fallback
-> container classifiers emit child document candidates when needed
-> parser emits normalized artifact metadata
-> retrieval ranks indexed artifacts and sections
```

This should improve indexed candidate quality and retrieval precision while preserving the local, deterministic, no-model architecture.

## Motivation

The current path-centered adapter approach has hit a product limit:

- relevant intent docs are invisible if their directory is not configured
- adding new default paths can look like fixture overfitting
- scan/index coverage and retrieval ranking failures are hard to separate
- ambiguous documents are treated as whatever adapter found them first

Classifier-backed adapters let DevSpecs use general document-processing signals:

- frontmatter
- headings and section roles
- status/lifecycle phrases
- checklist/task density
- filename and slug structure
- companion-file layout
- generated/stale/changelog negative evidence
- user-configured paths as hints, not hard truth

ADRs are the first document family where known subformats should improve accuracy. The classifier should recognize Nygard-style ADRs, MADR, and Y-Statement structure as ADR subformat evidence. RFCs and PRDs should start more conservatively as section-pattern families unless real samples show repeatable named templates.

## Goals

- Add a deterministic classifier contract with confidence, reasons, and negative reasons.
- Support both container-level classifiers and file/document-level classifiers.
- Support optional classifier subformats/families, starting with ADR Nygard, MADR, and Y-Statement evidence.
- Define a versioned classifier configuration structure for built-in profiles and repo/user overrides.
- Add a resolver that chooses a high-confidence adapter model or falls back to generic markdown.
- Add separate classifier eval metrics before classifier output is used for retrieval ranking.
- Keep path config as hints/overrides and backwards compatibility, not the core architecture.
- Support future real-sample evaluation from GitHub-mined files with provenance and license metadata.

## Non-Goals

- Do not add LLM, embedding, Ollama, OpenAI, Anthropic, or network calls.
- Do not broad-crawl every markdown file without hard filters and classifier rejection.
- Do not add a public `ds pack` command.
- Do not tune classifiers to seed fixture paths.
- Do not implement language-specific AST/source classifiers in this change.

## Related Plan

- `docs/plans/2026-05-14-adapter-classifier-pipeline-plan.md`
- `docs/plans/2026-05-13-retrieval-improvement-test-index.md`
- `openspec/changes/improve-retrieval-quality/`
- `openspec/changes/language-agnostic-intent-graph/`
