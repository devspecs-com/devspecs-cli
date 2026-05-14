# Design: Adapter Classifier Pipeline

## Problem

Adapters currently discover files mostly by path and then parse what they find. That makes path configuration a product bottleneck and makes eval failures harder to diagnose.

The new pipeline separates:

- candidate discovery
- universal feature extraction
- document classification
- container classification
- classification resolution
- parsing/indexing
- retrieval

This mirrors robust document-processing pipelines where classification is a measurable stage instead of an implicit side effect of file location.

## Pipeline

```text
Candidate Discovery
  -> Universal Feature Extraction
  -> Container and Document Classifier Set
  -> Resolver
  -> Container Expansion
  -> Parser
  -> Index
  -> Retrieval
```

### Candidate Discovery

Candidate discovery should gather plausible text documents from configured paths and general intent-doc conventions. It must apply hard filters before classification:

- ignored directories and files
- binary files
- oversized files
- generated outputs
- vendored dependencies
- lock files and build output

Candidate discovery emits reasons such as configured path, docs convention, root intent filename, or nested docs convention.

Candidate discovery and classification must support two scopes:

- `container`: directories or layouts that may contain multiple logical artifacts.
- `document`: standalone files that usually map to one logical artifact.

OpenSpec is the motivating container example: an `openspec/changes/<change>/` directory can contain `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md`, each of which should remain a retrievable child artifact. ADRs and PRDs are primitive document examples: each file usually maps to one artifact.

### Universal Feature Extraction

Universal features are extracted once and shared by classifiers:

- path tokens
- filename slug and date tokens
- extension and size
- frontmatter
- title and headings
- markdown sections
- checklist density
- status and lifecycle phrases
- identifier-shaped terms
- path-shaped references
- link targets
- generated/changelog/stale markers

The feature extractor should be deterministic and stack-neutral.

### Classifiers

Each classifier receives the same candidate and features, then returns a classification:

- classifier name
- scope: container or document
- optional subformat/family
- accepted/rejected
- confidence
- kind/subtype/status
- lifecycle and authority hints
- format profile and layout group
- extracted metadata
- positive reasons
- negative reasons
- child candidates when a container classifier expands a layout

Initial classifiers:

- OpenSpec container/document
- ADR
- PRD
- RFC/proposal
- Plan
- Agent note
- Generic markdown fallback

ADR classification should explicitly support common subformat families:

- Nygard-style ADRs with status, context, decision, and consequences.
- MADR-style ADRs with context/problem, decision drivers, considered options, decision outcome, and consequences.
- Y-Statement ADRs with the "in the context of..., facing..., we decided..., to achieve..., accepting..." structure.

These ADR families are confidence evidence and emitted subformats, not separate artifact kinds.

RFC and PRD classification should be more conservative. RFCs can use recurring engineering-proposal section families such as summary, motivation, proposal/detailed design, alternatives, risks, rollout, and open questions. PRDs should start with product-intent features such as goals, non-goals, personas, requirements, user stories, acceptance criteria, launch, and success metrics. Named RFC/PRD subformats should only be added after real samples show repeatable templates.

### Resolver

The resolver chooses a winner or fallback:

- strong accept when top confidence is high and separated
- ambiguous when top classifiers are close
- generic markdown fallback for useful but ambiguous text
- reject when all classifiers are weak and negative evidence is strong
- container winners can emit child document candidates before document classifiers run

Configured paths may add a prior but must not force a clearly wrong classification.

### Configuration

Classifier configuration should be versioned and layered:

- built-in classifier profile
- repo/user overrides

The built-in profile should support all documented models:

- OpenSpec
- OpenSpec container and child document roles
- ADR
- ADR Nygard subformat evidence
- ADR MADR subformat evidence
- ADR Y-Statement subformat evidence
- RFC/proposal section-pattern family
- PRD product-intent family
- plan families
- agent-note families
- generic markdown fallback
- local repo-defined model definitions

Configuration should allow:

- enabling or disabling models
- adding path hints
- adding local model definitions based on built-in models
- adjusting resolver thresholds in an advanced block
- controlling discovery breadth and hard filters

Configuration should not allow path hints to force a high-confidence classification when document features contradict the hint.

### Indexing

The first implementation should store classification metadata in existing extracted JSON fields to avoid premature schema work.

Only add normalized classifier tables after the prototype proves retrieval or explainability value.

### Retrieval

Retrieval should consume classifier output after classifier metrics are available. Initial uses:

- authority and lifecycle ranking
- artifact-type caps
- query-intent preferences
- OpenSpec companion expansion
- generic fallback penalty
- reason reporting

## Eval

Classifier eval is separate from retrieval eval.

Report:

- discovery coverage
- classifier accuracy by model
- false positives
- false negatives
- ambiguity rate
- generic fallback rate
- reject rate
- top confusion pairs
- reason coverage

Retrieval eval continues to report:

- token reduction
- overall recall
- must-have recall
- precision
- sufficiency

## Real Samples

Real GitHub samples may be used if provenance is recorded:

- source URL
- repository
- commit SHA
- license
- original path
- expected format label
- whether the file can be committed

If redistribution is unclear, keep the real file outside the repo and commit a reduced synthetic derivative instead.

## Risks

- Broad discovery can increase noise.
- Classifiers can encode fragile conventions.
- Confidence scores can look more precise than they are.
- Negative terms can hide legitimate docs.
- Real samples can introduce licensing ambiguity.

Mitigations:

- keep hard filters conservative
- keep classifier reasons auditable
- use generic fallback for ambiguity
- evaluate classifier quality separately from retrieval
- avoid fixture-specific paths and keywords
- track provenance for mined samples
