---
title: Language-Agnostic Intent Graph Plan
kind: plan
status: draft
tags: [retrieval, intent-graph, indexing, heuristics, multi-stack]
---

# Language-Agnostic Intent Graph Plan

## Context

DevSpecs should not overfit retrieval to TypeScript, React, Go, or any single stack. The seed fixture happens to use TypeScript/React plus API code because it is a realistic SaaS shape, but the product claim is broader:

DevSpecs prepares compact agent context from repo intent artifacts across repo and stack boundaries.

The next retrieval architecture should borrow the useful local-first ideas from Kleio without importing Kleio's goal wholesale. Kleio cross-correlates engineering intent signals across plans, commits, transcripts, and history. DevSpecs indexes existing intent artifacts and packs targeted context. The shared useful idea is a deterministic local graph of artifacts, entities, and relationships.

Related OpenSpec change: `openspec/changes/language-agnostic-intent-graph/`.

## North Star

Improve must-have recall, precision, and context sufficiency without relying on LLMs, model calls, or language-specific assumptions.

The retrieval model should be:

- Language-agnostic by default.
- Stack-aware only through optional adapters.
- Explainable in eval output.
- Measured against token reduction, recall, precision, and sufficiency.

## Design Position

AST extraction can help, but it should not be the core abstraction.

The core abstraction should be:

```text
artifact -> section -> entity -> edge -> retrieval packet
```

ASTs, regex identifier extraction, OpenSpec parsing, ADR metadata, markdown headings, and source paths are all evidence sources feeding the same artifact graph.

This avoids a trap where DevSpecs becomes excellent at TypeScript exports but weak on Python services, Go CLIs, Rails apps, Terraform modules, docs-heavy repos, or mixed monorepos.

## What To Borrow From Kleio

### Entity Normalization

Kleio already extracts and normalizes tickets, plan anchors, file paths, and decision-like headings. DevSpecs should adapt that idea for intent artifacts:

- file paths
- source identifiers
- artifact slugs
- OpenSpec change IDs
- ADR numbers
- PRD names
- headings
- task IDs
- status and lifecycle labels

Normalization should preserve exact identifier forms while also deriving useful variants:

- `stripe_event_id`
- `stripe-event-id`
- `stripe event id`
- `stripe.event.id`
- `StripeEventID`

Audit target:

- Identifier-shaped eval cases improve must-have recall without widening retrieval to every generic billing/auth/customer artifact.

### Section Boundary Extraction

Kleio treats plans as structured signals: umbrella goal, todos, decisions, deferred work, risks, and rationale. DevSpecs should use a similar approach for markdown artifacts:

- frontmatter
- headings
- checklists
- decision/rationale blocks
- risks/open questions
- deferred/out-of-scope sections
- acceptance/success criteria
- status/lifecycle sections

This lets retrieval score specific sections before deciding whether to include the whole artifact, a section excerpt, or a bundled companion artifact.

Audit target:

- A query about stale local entitlement caching should match superseded/stale sections without pulling active implementation plans as if they were current.

### Alias And Co-Occurrence Learning

Kleio learns aliases from repeated co-occurrence and substring evidence. DevSpecs can use a bounded deterministic version:

- exact alias from frontmatter/title/slug
- identifier variant alias
- co-occurrence alias only when the same pair appears across enough independent artifacts or sections
- no global synonym expansion without local evidence

Examples:

- `webhook replay protection` <-> `webhook_replay_protection`
- `event id` <-> `stripe_event_id`, only when local evidence supports it
- `entitlement sync` <-> `harden-entitlement-sync`

Audit target:

- Alias expansion should improve recall in identifier and slug cases while preserving or improving precision.

### Correlation Edges

Kleio has deterministic correlators for ID references, file paths, search similarity, and time windows. DevSpecs should start with static filesystem-friendly edges:

- `contains_section`
- `mentions_entity`
- `belongs_to_openspec_change`
- `openspec_companion`
- `references_path`
- `same_slug_or_alias`
- `explains_decision`
- `constrains_implementation`
- `supersedes`
- `marked_stale`
- `product_background_for`

Later, git/session history can add time and implementation edges, but v0 should remain deterministic and local, with indexed eval as the product-adjacent control.

Audit target:

- OpenSpec implementation context retrieves proposal/design/tasks/spec deltas as explainable companion edges, not as case-specific weight hacks.

### Multi-Factor Ranking

Kleio's ranking combines entity overlap, file overlap, link strength, recency, and structural proximity. DevSpecs should adapt the scoring dimensions to static intent artifacts:

- query/entity overlap
- exact identifier match
- artifact authority
- lifecycle compatibility
- edge/link strength
- section role match
- source/path overlap
- artifact bundle membership
- noise penalty

This is more robust than a weighted bag of terms because generic terms like `billing`, `auth`, `customer`, `token`, and `webhook` can be saturated or downweighted.

Audit target:

- Precision improves because low-authority generic matches stop crowding out must-have artifacts.

## Language-Agnostic Extraction Layers

### Layer 1: Universal Text And Path Extraction

This should work for any repo:

- tokenize paths, filenames, slugs, and extensions
- preserve snake_case, kebab-case, dotted names, slash paths, and CamelCase variants
- parse markdown headings and checklists
- parse YAML frontmatter when present
- detect common status labels: active, proposed, accepted, superseded, stale, archived, ready-to-archive
- detect file paths inside prose
- detect identifier-shaped terms with language-neutral regexes

This is the first implementation target.

### Layer 2: Artifact-Type Adapters

These are not programming-language-specific:

- OpenSpec adapter: proposal/design/tasks/spec deltas and change bundles
- ADR adapter: decision status, context, consequences, supersession
- PRD adapter: product requirements, user-facing constraints, rollout/background
- Plan adapter: task/checklist/decision/deferred/risk sections
- Agent-note adapter: follow-up, continuation, blocker, implementation hints

This is where DevSpecs should get the largest retrieval gain for the current product.

### Layer 3: Optional Source Symbol Adapters

These should be additive and swappable, not required:

- generic regex symbol extraction for common code identifiers
- tree-sitter adapters where available
- language-specific exporters later, such as Go funcs/types, Python defs/classes, Java/Kotlin classes, C# classes/methods, Rust items, JS/TS exports

The retrieval API should not expose "TypeScript symbol" as a core concept. It should expose "source_symbol" or "code_entity" with a language/source field.

Audit target:

- A repo with no supported AST parser still gets useful retrieval from universal text/path/entity extraction.

## Proposed Retrieval Flow

1. Collect candidate artifacts.
2. Parse artifact metadata and sections.
3. Extract normalized entities.
4. Build deterministic edges between artifacts, sections, and entities.
5. Classify query intent using local heuristics.
6. Expand query entities using local aliases and variants.
7. Score candidate sections and artifacts.
8. Apply bundle expansion for strong artifact relationships.
9. Pack context under token budget.
10. Emit reasons for every included artifact.

## Eval Additions

The current seed eval can measure this without LLMs by adding or refining cases for:

- language-neutral identifier variants
- path-shaped terms across multiple extensions
- OpenSpec companion edges
- ADR supersession edges
- PRD background versus implementation context
- generic noisy terms that should saturate
- artifact section roles such as decision, deferred, risk, task, and rationale
- source candidates that are relevant by identifier but not by language

Do not add a TypeScript-only success target. Add one or more language-neutral source examples over time, such as:

- Go file with exported function/type
- Python file with class/function
- SQL migration
- Terraform module or YAML workflow

These should test the shared extraction model, not stack-specific cleverness.

## Auditable Success Criteria

The plan is successful when a reviewer can verify:

- The index model has language-neutral artifact, section, entity, and edge concepts.
- Source-symbol extraction is optional and represented generically.
- Retrieval reasons identify normalized entity matches, section matches, and edge expansion.
- Eval output shows before/after movement on:
  - token reduction
  - overall recall
  - must-have recall
  - precision
  - sufficiency pass rate
- At least one eval case proves useful source-context retrieval without relying on TypeScript-only behavior.
- No LLM, embedding provider, network service, or model call is required.

## Guardrails

- Do not make TypeScript, React, or Node terminology first-class retrieval concepts.
- Do not add a graph database dependency for v0.
- Do not use learned aliases unless they are explainable and derived from local evidence.
- Do not let broad co-occurrence aliases make every billing/auth/customer artifact relevant.
- Do not use AST availability as a hard requirement for source-context retrieval.
- Do not claim semantic understanding beyond deterministic local evidence.

## Work Order

1. Define in-memory artifact graph types.
2. Add universal entity normalization and extraction.
3. Add section extraction for markdown and frontmatter.
4. Add OpenSpec, ADR, PRD, plan, and agent-note adapters.
5. Add deterministic edge construction.
6. Add query intent classification over graph entities.
7. Add graph-aware scoring and bundle expansion.
8. Add explainable retrieval reasons.
9. Add language-neutral eval cases.
10. Compare saved timestamped eval results before and after each step.

## Expected Outcome

The first win should not be a fancy AST-powered semantic search engine. It should be a more credible local retrieval system that understands repo intent structure:

- active design beats stale scratch for implementation context
- superseded ADR wins for stale-history queries
- OpenSpec companions travel together when appropriate
- exact identifiers beat generic topic matches
- product background queries can include PRDs without polluting implementation context

That is the path toward the north star: compact context with the right intent preserved across many repo shapes.
