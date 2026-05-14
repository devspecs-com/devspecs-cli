# Design: Language-Agnostic Intent Graph

## Problem

The current eval retriever is mostly a weighted file scorer. It can compress context, but it lacks a durable representation for why artifacts relate to each other. That makes it easy to miss companion artifacts, overselect generic distractors, and overfit retrieval improvements to visible cases or fixture language.

## Core Model

DevSpecs should build a deterministic graph from local repo evidence:

```text
Artifact
  has many Sections
  mentions many Entities
  connects through Edges
```

### Artifact

An artifact is a file or logical document:

- OpenSpec proposal/design/tasks/spec delta
- ADR
- PRD
- markdown plan
- Cursor or Claude note
- source file
- migration/config/workflow file

### Section

A section is a meaningful span inside an artifact:

- frontmatter
- heading body
- checklist item
- decision block
- rationale block
- deferred/out-of-scope block
- risk/open-question block
- success/acceptance criteria

Sections are useful because scoring the entire file hides the difference between "this file mentions billing once" and "this section is the active implementation task list."

### Entity

An entity is a normalized local concept:

- path
- identifier
- slug
- OpenSpec change ID
- ADR ID
- PRD name
- heading title
- task ID
- lifecycle/status label
- source symbol, when available

Source symbols are generic entities. A TypeScript export, Go function, Python class, SQL object, or Terraform resource should all fit under source/code entity kinds.

### Edge

Edges are deterministic relationships:

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

Edges should include a reason and confidence so eval output can explain retrieval choices.

## Extraction Layers

### Layer 1: Universal Extraction

Works for every repo:

- path and filename tokenization
- markdown heading extraction
- YAML frontmatter extraction
- checkbox/task extraction
- identifier-shaped term extraction
- path-shaped prose extraction
- status/lifecycle labels
- slug and dated filename parsing

### Layer 2: Artifact-Type Adapters

Adapters understand intent artifact formats, not programming languages:

- OpenSpec adapter links proposal/design/tasks/spec deltas.
- ADR adapter extracts status, supersession, context, decision, consequences.
- PRD adapter extracts product requirements and background.
- Plan adapter extracts task, decision, deferred, risk, rationale sections.
- Agent-note adapter extracts follow-up, blocker, continuation, and implementation hints.

### Layer 3: Optional Source Adapters

Source adapters are additive:

- regex identifier extraction across text/code files
- tree-sitter where available
- language-specific symbol extraction later

The absence of a source adapter must not prevent useful retrieval.

## Query Flow

1. Normalize query terms and identifier variants.
2. Classify query intent, such as implementation context, product background, stale history, source identifier lookup, or resume work.
3. Expand query entities through local aliases and variants.
4. Score sections using entity overlap, exact identifier matches, role, lifecycle, and artifact authority.
5. Promote artifacts from high-scoring sections.
6. Expand through strong graph edges, such as OpenSpec companions.
7. Apply token budget and noise caps.
8. Emit retrieval reasons.

## Why Not AST First

ASTs are useful for source files, but they are not the main structure of repo intent. Many decisive artifacts are markdown, OpenSpec, ADRs, PRDs, SQL migrations, YAML config, issue plans, and agent notes.

An AST-first design risks making DevSpecs good at one stack while missing the actual intent layer. A graph-first design lets AST extraction become one evidence source among many.

## Eval Strategy

The existing seed eval should add language-neutral cases before any marketing claim:

- exact identifier retrieval across source and docs
- path-shaped retrieval across different file extensions
- OpenSpec companion expansion
- ADR supersession/lifecycle selection
- PRD background versus implementation context
- source/context candidates that are not TypeScript-specific

The eval should keep reporting token reduction, overall recall, must-have recall, precision, and sufficiency.

## Risks

- Co-occurrence aliasing can overgeneralize and hurt precision.
- Graph expansion can bloat context if bundle rules are too broad.
- Section extraction can create false confidence if headings are generic.
- Language-specific adapters can quietly become product assumptions.

Mitigations:

- Keep aliases local, explainable, and thresholded.
- Cap graph expansion by query intent and artifact authority.
- Penalize generic headings.
- Require language-neutral eval cases.
