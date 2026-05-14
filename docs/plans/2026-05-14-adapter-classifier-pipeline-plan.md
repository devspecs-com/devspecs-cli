---
title: Adapter Classifier Pipeline Plan
kind: plan
status: draft
tags: [adapters, classification, indexing, retrieval, eval]
---

# Adapter Classifier Pipeline Plan

## Purpose

Move DevSpecs away from path configuration as the center of indexing and toward deterministic document classification.

Path config should remain useful for:

- user overrides
- performance hints
- repo-specific conventions
- backwards compatibility

But the product should not require users or eval fixtures to enumerate every intent-artifact directory. The stronger architecture is:

```text
broad safe candidate discovery
-> universal document feature extraction
-> declarative document models score candidates
-> classifier resolver chooses a document model
-> parser emits normalized artifact metadata
-> retrieval ranks normalized artifacts and sections
```

This is still deterministic and local-only. No LLMs, embeddings, network calls, or hosted model APIs are required.

## Branch And Commit Discipline

Implementation work for this plan should happen on:

```text
feat/adapter-classifier-pipeline
```

Agents should commit incrementally to that branch as they complete coherent slices of work. A good commit boundary is one auditable phase or experiment, such as:

- Phase 0 contracts and goldens
- universal feature extraction
- one classifier model plus tests
- classifier eval reporting
- scan integration prototype
- retrieval integration experiment with saved eval results

Each commit should include the relevant plan/OpenSpec updates and any saved eval result paths when behavior changes. Avoid large mixed commits that combine schema, ranking, fixtures, and docs without a clear before/after.

## Current Problem

The current adapter interface mixes three responsibilities:

```go
Discover(repo) -> []Candidate
Parse(candidate) -> Artifact
```

That means an adapter usually decides whether a document matters by knowing where to look. This creates three product risks:

- Relevant files are invisible if their paths are not configured or guessed.
- Adding default paths can look like fixture overfitting.
- Retrieval quality is confounded with scan/index coverage.

The current eval numbers reflect this. The indexed path has improved after general `docs/prd` and nested `*/docs/plans` coverage, but the remaining failures show that candidate discovery, document typing, lifecycle metadata, source candidates, and ranking need to be measured separately.

## Document Processing Principles

Use a staged pipeline common to robust local document processing systems:

1. **Acquisition:** enumerate plausible local files without interpreting them.
2. **Filtering:** reject ignored, binary, generated, oversized, vendored, and low-value files early.
3. **Feature Extraction:** extract cheap universal signals once.
4. **Classification:** let declarative container and document models score candidates with positive and negative evidence.
5. **Resolution:** choose container/document models, fall back when ambiguous, and preserve reasons.
6. **Expansion and Parsing:** container models can emit child document candidates; document parsers produce normalized artifacts, sections, entities, lifecycle, and authority.
7. **Indexing:** persist normalized output plus classifier evidence.
8. **Retrieval:** rank artifacts and sections using query intent, entity overlap, authority, lifecycle, and graph edges.
9. **Evaluation:** measure discovery coverage, classifier quality, retrieval quality, and context sufficiency independently.

## Target Components

### Candidate Discovery

Candidate discovery should be broad but safe.

Inputs:

- configured source paths
- known intent-doc conventions
- root-level intent filenames
- nested `*/docs/{plans,prd,specs,design,technical}` style directories
- optional broad markdown discovery after classifier filters exist
- later: source/context candidate discovery for code, SQL, config, workflow, and migration files

Hard filters:

- ignore `.git`, `node_modules`, lock files, build outputs, vendored dependencies, and ignored paths
- skip binary files
- cap file size
- cap total candidate count per repo unless configured
- avoid generated API docs, dependency docs, and release/changelog-only files unless explicitly configured

Candidate discovery should emit `DiscoveryReason` values so eval can explain why a file entered the classifier pipeline.

Discovery must support two candidate scopes:

- **Container candidates:** directories or layouts that can contain multiple logical artifacts. Example: an OpenSpec change directory with `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md`.
- **Document candidates:** standalone files that usually become one logical artifact. Example: one ADR, one PRD, one RFC, one plan, one agent note.

The pipeline should avoid forcing every model into one scope. Some formats are naturally containers, some are primitive documents, and some may support both.

### Universal Feature Extraction

Extract once for every candidate:

- path tokens
- filename slug tokens
- dated filename tokens
- extension and rough media type
- frontmatter keys and values
- title and heading outline
- section spans by markdown heading
- checklist density
- status/lifecycle phrases
- identifier-shaped terms
- path-shaped references
- link targets
- code-fence languages
- repeated high-signal local terms
- generated-file markers
- changelog/release-note markers
- stale/scratch/deprecated markers

These features should be available to all document models and to retrieval reasons.

### Classifier Contract

Classifier behavior should be represented as declarative document-model configuration: stable evidence rule IDs, feature predicates, weights, negative evidence, and resolver thresholds. The Go implementation should be a generic evaluator for that configuration rather than a hard-coded classifier per document type.

This is intentional because we want to mine and tune weights from real sample files with automated statistical regressions and sample test sets. If evidence is buried in Go branches, we cannot audit feature contribution, compare fitted weights, or reject brittle patterns cleanly.

Introduce a package such as:

```text
internal/classify
```

Suggested types:

```go
type Candidate struct {
    Path        string
    Scope       string // container | document
    Ext         string
    SizeBytes   int64
    Body        string
    Features    Features
    SourceHints []SourceHint
}

type Classifier interface {
    Name() string
    Classify(Candidate) Classification
}

type Classification struct {
    Classifier      string
    Scope           string // container | document
    Subformat       string
    Accepted        bool
    Confidence      float64
    Kind            string
    Subtype         string
    Status          string
    Lifecycle       string
    Authority       string
    FormatProfile   string
    LayoutGroup     string
    PositiveReasons []Reason
    NegativeReasons []Reason
    ChildCandidates []Candidate
    Extracted       map[string]any
}

type Resolution struct {
    Winner          Classification
    Alternatives    []Classification
    Ambiguous       bool
    FallbackGeneric bool
}
```

The exact names can change during implementation, but the contract should preserve:

- confidence
- candidate scope: `container` or `document`
- optional subformat/family
- positive reasons
- negative reasons
- accepted/rejected state
- fallback/ambiguity
- normalized kind/subtype/status/lifecycle/authority
- child document candidates emitted by container document models

### Classifier Scopes

#### Container Document Models

Container document models evaluate directories or multi-file layouts. They can emit multiple child document candidates and layout-level edges.

Examples:

- OpenSpec change directory
- future multi-file RFC/proposal packages
- future generated plan bundles, if real samples justify them

Container classifier output should include:

- container ID or layout group
- child candidates with roles
- bundle/companion edges
- shared lifecycle/status when applicable
- container-level reasons

#### Primitive Document Models

Primitive document models evaluate standalone files. They usually emit one artifact.

Examples:

- ADR
- PRD
- RFC/proposal markdown file
- implementation plan
- agent note
- generic markdown fallback

Document classifier output should include:

- document model
- optional subformat/family
- normalized metadata
- section roles
- positive and negative reasons

#### Hybrid Models

Some future models may support both scopes. They should declare supported scopes explicitly and avoid silently treating a container as a single document when child artifacts are expected.

### Resolver

The resolver compares classifier outputs.

Initial deterministic rules:

- Strong accept when top confidence is above a threshold and separated from the next classifier.
- Ambiguous when the top score is weak or the top two document models are close.
- Generic markdown fallback when ambiguous but the file is still useful text.
- Reject when all document models are low-confidence and negative evidence is strong.
- User-configured paths can add a prior, but should not force a bad classification by themselves.

Example thresholds for planning, not implementation gospel:

```text
strong_accept >= 0.75
weak_accept >= 0.55
ambiguity_gap < 0.15
reject < 0.35 with strong negative evidence
```

Thresholds should be calibrated with classifier goldens, not tuned to make one eval case pass.

## Classifier Configuration Structure

Classifier configuration should support every documented document model while keeping the default user experience simple.

Use two layers:

- **Built-in classifier profile:** versioned defaults maintained by DevSpecs.
- **Repo/user overrides:** small, explicit overrides for enabling/disabling models, adding hints, adjusting thresholds, or defining local conventions.

Avoid exposing raw scoring weights as the primary user interface. Most users should configure paths and local conventions, not classifier internals.

### Proposed Shape

This shape is illustrative, but the concepts should survive implementation:

Document-model behavior is expressed with reusable evidence rules:

```yaml
evidence:
  - id: adr_nygard_sections
    weight: 0.26
    reason: subformat_evidence
    match:
      scope: document
      headings_all: [Context, Decision, Consequences]
negative_evidence:
  - id: plan_generated_marker
    weight: 0.24
    reason: generated_marker
    match:
      markers_any: [generated]
```

Supported predicate families include path hints/globs, title terms, frontmatter keys and values, heading patterns, section roles, checklist density, date tokens, markers, identifiers, local terms, body terms/regexes, and container child roles.

```yaml
classifier_pipeline:
  version: 1
  profile: builtin_intent_docs_v1

  discovery:
    mode: conservative
    include_configured_sources: true
    include_known_intent_conventions: true
    include_nested_docs_conventions: true
    broad_markdown_discovery: false
    max_file_size_bytes: 262144
    max_candidates: 2000
    ignore_generated: true
    ignore_vendored: true
    extra_include_globs: []
    extra_exclude_globs: []

  resolver:
    strong_accept: 0.75
    weak_accept: 0.55
    ambiguity_gap: 0.15
    reject_below: 0.35
    fallback: generic_markdown
    configured_path_prior: 0.10
    configured_path_can_force: false

  models:
    openspec:
      enabled: true
      scopes: [container, document]
      authority: high_current_intent
      path_hints:
        - openspec/changes/**
      layout_hints:
        proposal: proposal.md
        design: design.md
        tasks: tasks.md
        specs: specs/**/spec.md
      emits_edges:
        - openspec_companion

    adr:
      enabled: true
      scopes: [document]
      authority: high_decision
      path_hints:
        - docs/adr/**
        - docs/adrs/**
        - adr/**
        - adrs/**
      subformats:
        nygard:
          enabled: true
        madr:
          enabled: true
        y_statement:
          enabled: true

    rfc:
      enabled: true
      scopes: [document]
      authority: design_proposal
      path_hints:
        - rfcs/**
        - docs/rfcs/**
        - docs/proposals/**
      families:
        section_pattern:
          enabled: true
      named_subformats: []

    prd:
      enabled: true
      scopes: [document]
      authority: product_background
      path_hints:
        - docs/prd/**
        - docs/prds/**
        - prd/**
        - prds/**
      families:
        product_intent:
          enabled: true
      named_subformats: []

    plan:
      enabled: true
      scopes: [document]
      authority: working_plan
      path_hints:
        - plans/**
        - docs/plans/**
      families:
        implementation_plan:
          enabled: true
        migration_plan:
          enabled: true
        rollout_plan:
          enabled: true

    agent_note:
      enabled: true
      scopes: [document]
      authority: handoff_note
      path_hints:
        - .cursor/plans/**
        - .claude/**
        - .codex/**
      families:
        continuation_note:
          enabled: true
        followup_note:
          enabled: true
        blocker_note:
          enabled: true

    generic_markdown:
      enabled: true
      scopes: [document]
      fallback: true
      authority: neutral

  local_models:
    # Optional repo-defined document models can add hints and labels, but should not
    # bypass resolver confidence/ambiguity rules by default.
    enabled: true
    definitions: []
```

### User Override Examples

Most repo configs should need only small overrides:

```yaml
classifier_pipeline:
  models:
    rfc:
      path_hints:
        - architecture/rfcs/**
        - proposals/**
    prd:
      path_hints:
        - product/requirements/**
    agent_note:
      enabled: false
```

Local document families can be represented without changing built-ins:

```yaml
classifier_pipeline:
  local_models:
    definitions:
      - id: engineering_brief
        base_model: rfc
        authority: design_proposal
        path_hints:
          - briefs/**
        required_headings:
          - Problem
          - Proposal
        positive_terms:
          - rollout
          - tradeoff
        negative_terms:
          - changelog
```

### Supported Models

The configuration structure must support:

- `openspec`
- `openspec.container`
- `openspec.document`
- `adr`
- `adr.nygard`
- `adr.madr`
- `adr.y_statement`
- `rfc`
- `rfc.section_pattern`
- `prd`
- `prd.product_intent`
- `plan`
- `plan.implementation_plan`
- `plan.migration_plan`
- `plan.rollout_plan`
- `agent_note`
- `agent_note.continuation_note`
- `agent_note.followup_note`
- `agent_note.blocker_note`
- `generic_markdown`
- repo-defined `local_models`

### Configuration Guardrails

- Built-in defaults must work without config.
- User config can add hints and disable models.
- User config can adjust resolver thresholds only in an advanced block.
- Path hints add confidence but do not force a classification.
- Local models inherit a built-in base model unless explicitly marked experimental.
- Negative hints must be auditable in classifier reasons.
- Configured terms must appear in classifier output reasons when they influence a decision.
- Config schema should be versioned independently from repo config version if needed.

## Adapter Models

### OpenSpec Document Model

Scope:

- primarily `container`
- may classify individual OpenSpec child documents after container expansion

Positive signals:

- path/layout resembles `openspec/changes/<change>/proposal.md`
- companion files `design.md`, `tasks.md`, `specs/**/spec.md`
- headings like proposal, motivation, impact, tasks, requirements
- change-id slug reused across files

Negative signals:

- standalone markdown with no OpenSpec layout
- changelog/release notes using similar terms

Emits:

- container classification for `openspec/changes/<change>/`
- child document candidates for proposal/design/tasks/spec deltas
- `kind: spec` or plan/design depending on file role
- `format_profile: openspec`
- layout group for the change directory
- companion edge candidates

### ADR Document Model

Scope:

- `document`

Positive signals:

- ADR numbering or decision-record path
- status line
- context/decision/consequences headings
- supersedes/superseded-by markers

Known ADR subformat families should improve confidence when present:

- `adr.nygard`: status plus `Context`, `Decision`, and `Consequences` sections.
- `adr.madr`: MADR-style sections such as `Context and Problem Statement`, `Decision Drivers`, `Considered Options`, `Decision Outcome`, and positive/negative consequences.
- `adr.y_statement`: Y-Statement structure such as "In the context of ..., facing ..., we decided ..., to achieve ..., accepting ..."

Subformats are evidence, not separate top-level artifact kinds. An ADR can still classify as ADR without matching one of these families.

Negative signals:

- random design docs that mention "decision" once
- meeting notes with decision-like sections but no durable ADR structure

Emits:

- `kind: decision`
- optional `subformat: adr.nygard | adr.madr | adr.y_statement`
- lifecycle/status
- authority high for accepted decisions
- supersession metadata

### PRD Document Model

PRDs should start as a product-intent feature classifier rather than a list of named subformats. Real samples may later reveal recurring templates, but v0 should avoid pretending there is one universal PRD standard.

Scope:

- `document`

Positive signals:

- product requirements language
- goals, non-goals, personas, users, success metrics
- requirements, acceptance criteria, rollout, scope
- `prd` frontmatter, filename, or directory hints

Negative signals:

- implementation runbooks
- broad billing notes with no product requirement structure
- changelog/release notes

Emits:

- `kind: requirements`
- `subtype: prd`
- product-background authority

### RFC Document Model

RFCs and engineering proposals should use section-pattern family evidence rather than strict named standards.

Scope:

- `document` for v0
- possible `container` later if real samples show multi-file RFC packages

Likely positive section families:

- summary or abstract
- motivation/problem statement
- proposal or detailed design
- alternatives considered
- drawbacks/risks
- compatibility/migration/rollout
- unresolved questions/open questions
- security/privacy/operational impact

Rust-style RFC sections, API-change RFC sections, and internal engineering-proposal sections can be recognized as families if real samples justify them. Until then, RFC classification should remain conservative.

Negative signals:

- package README files
- changelogs or release notes
- implementation plans with no proposal/rationale structure
- product PRDs with user/persona/requirement structure but no engineering proposal

Emits:

- `kind: design` or `kind: spec` depending on repository convention and confidence
- optional `subformat: rfc.section_pattern`
- design/proposal authority

### Plan Document Model

Scope:

- `document`

Positive signals:

- dated plan filename
- implementation plan headings
- checklists and task lists
- risks, open questions, phased work
- migration/rollout/implementation language

Negative signals:

- generated task output
- broad support runbook
- changelog
- scratch/stale markers unless classifier emits low authority lifecycle

Emits:

- `kind: plan`
- section roles for tasks, risks, open questions, deferred work

### Agent Note Document Model

Scope:

- `document`

Positive signals:

- Cursor, Claude, Codex, or other agent-note path/profile hints
- continuation, follow-up, blocker, handoff, next step language
- implementation notes tied to known artifacts

Negative signals:

- chat transcripts without durable plan content
- broad notes with no actionable context

Emits:

- `kind: plan` or `markdown_artifact` depending on confidence
- `format_profile` for the agent/tool when deterministic
- lower authority than ADR/OpenSpec by default

### Generic Markdown Document Model

Generic markdown is not failure. It is the safe fallback.

Scope:

- `document`

Use it when:

- a document is useful text but does not strongly match a known model
- multiple document models are close
- path hints and content features disagree

Emits:

- `kind: markdown_artifact`
- low or neutral authority
- extracted headings and identifiers

## Eval Strategy

Add classifier-specific evals before using classifier output to tune retrieval.

Suggested `cases.yaml` additions or a new classifier fixture shape:

```yaml
classifier_cases:
  - id: adr-accepted-boundary
    path: docs/adr/0002-webhook-idempotency-boundary.md
    expected:
      classifier: adr
      kind: decision
      status: accepted
      authority: high
      should_index: true

  - id: ambiguous-billing-notes
    path: docs/plans/2026-04-customer-portal-billing-notes.md
    expected:
      classifier: plan
      authority: low
      should_index: true
      must_not_classify_as: prd
```

Report:

- candidate discovery coverage
- classifier accuracy by model
- false positives by model
- false negatives by model
- ambiguity rate
- generic fallback rate
- reject rate
- top confusion pairs
- reason coverage
- retrieval metrics after classifier metadata is consumed

The eval should keep writing timestamped JSON result files.

## Real Sample Intake

If real GitHub samples are mined, store them with provenance and licensing discipline.

For each sample:

- source URL
- repository
- commit SHA
- license
- original path
- format label
- whether the full file can be committed
- any reduction/synthetic rewrite notes

Prefer one of these paths:

- commit license-compatible full files under `testdata/classifier-samples/real/`
- commit reduced synthetic derivatives under `fixtures/classifier-docs/`
- keep non-redistributable samples outside the repo and record only metadata

Do not let real-sample labels become hidden training data for fixture-specific rules. Classifier changes should cite the feature pattern, not the individual file.

## Migration Plan

### Phase 0: Contract And Goldens

Status: implemented

Deliverables:

- define classifier result types
- define classifier configuration schema and built-in profile shape
- define container versus document candidate scope
- define reason vocabulary
- define classifier fixture format
- add first classifier golden cases from current fixture
- add real-sample intake notes

Success criteria:

- [x] classifier tests can run without changing scan behavior
- [x] no DB/schema migration required
- [x] no retrieval metric claim yet

Result:

- Added `internal/classify` with Phase 0 contracts only.
- Added default built-in classifier pipeline config for all documented models and subformat/family models.
- Added classifier golden-case loader and validator.
- Added seed fixture classifier goldens at `fixtures/agentic-saas-fragmented/classifier_cases.yaml`.
- Added real-sample provenance template at `testdata/classifier-samples/provenance-template.yaml`.

### Phase 1: Universal Feature Extractor

Status: implemented

Deliverables:

- feature extractor for markdown/text candidates
- path, frontmatter, headings, sections, identifiers, lifecycle, generated markers
- deterministic unit tests

Success criteria:

- [x] feature extraction has deterministic tests
- [x] extraction is stack-neutral
- [x] extraction remains disconnected from scan/retrieval integration

Result:

- Added `ExtractFeatures` and `EnrichCandidate` in `internal/classify`.
- Extracts path tokens, filename tokens, dated tokens, frontmatter, title, headings, section spans/roles, checklist counts, status/lifecycle phrases, identifier-shaped terms, path references, links, code fence languages, local repeated terms, and generated/changelog/stale markers.
- Added feature extraction unit tests without changing scan, DB, CLI, or retrieval behavior.

### Phase 2: Declarative Document Model Evaluator

Status: implemented

Deliverables:

- generic evidence-rule evaluator over built-in model configuration
- declarative OpenSpec, ADR, PRD, RFC/proposal, plan, agent-note, and generic markdown model rules
- declarative ADR Nygard, MADR, and Y-Statement subformat evidence
- declarative PRD/RFC/plan/agent-note family evidence
- resolver for confidence, ambiguity, and generic fallback
- local model definitions that inherit built-in models and add declarative evidence
- deterministic unit tests for goldens, subformats, fallback, negative evidence, and local models

Success criteria:

- [x] document-model rules are data in `PipelineConfig`, not hard-coded per-type branches
- [x] ambiguous documents fall back instead of being overclaimed
- [x] negative reasons are visible
- [x] rule IDs and weights are auditable for future sample-based fitting

Result:

- Added `ClassifyCandidate` as a generic declarative evaluator in `internal/classify`.
- Extended `PipelineConfig` with `EvidenceRule` and `EvidenceMatch`.
- Added default evidence rules for all documented built-in models and subformat/family models.
- Added local model evaluation that inherits a base model and adds declarative local evidence.
- Kept scan, DB, CLI, and retrieval behavior unchanged.

### Phase 3: Classifier Eval

Status: implemented

Deliverables:

- focused classifier eval mode
- model accuracy by document model
- subformat/family accuracy where expected labels exist
- false positives and false negatives by model
- ambiguity rate
- generic fallback rate
- reject rate
- top confusion pairs
- reason coverage
- timestamped classifier eval JSON

Success criteria:

- [x] classifier eval runs without scan, DB, retrieval, network, or model calls
- [x] classifier eval labels the evaluator and classifier profile
- [x] classifier eval reports confusion, fallback, ambiguity, rejection, and reason metrics
- [x] classifier eval saves timestamped JSON by default through `ds eval --classifier`

Result:

- Added `ds eval <fixture> --classifier`.
- Added `internal/classify.RunEval` with text and JSON formatters.
- Added per-case classifier outcomes, per-model precision/recall, subformat/family accuracy, generic fallback rate, ambiguity rate, reject rate, reason coverage, and child-candidate coverage.
- The current seed smoke classifier goldens are intentionally small and currently pass 6/6; this validates the harness, not a locked benchmark.

### Phase 4: Scan Pipeline Integration

Status: planned

Deliverables:

- broad safe candidate discovery
- classifier resolver before parse/index
- existing configured adapters remain compatible
- classification metadata stored in `Artifact.Extracted` first

Success criteria:

- `ds scan` remains deterministic
- current configured-path behavior still works
- indexed eval does not regress catastrophically

### Phase 5: Retrieval Consumption

Status: planned

Deliverables:

- retrieval candidates include classifier, authority, lifecycle, ambiguity, section roles
- ranking uses classifier metadata before broad text hits
- eval reasons show classifier influence

Success criteria:

- precision improves without reducing must-have recall materially
- sufficiency improves in at least one implementation-context case
- no new public CLI command is required

### Phase 6: Schema Hardening

Status: deferred

Deliverables:

- persist classifier results in normalized tables if `Extracted` becomes insufficient
- persist classifier alternatives and confidence
- persist section/entity/edge tables if graph retrieval is ready

Success criteria:

- only do this after the JSON/extracted prototype proves value
- migrations are justified by measured retrieval or explainability wins

## Guardrails

- No LLM or embedding dependency.
- No fixture-specific paths or keywords.
- No TypeScript-only assumptions.
- No silent classifier decisions without reasons.
- No broad repo-wide markdown crawl until hard filters and classifier rejection are tested.
- No marketing claim until live-command eval improves on locked fixtures.
- Prefer generic markdown fallback over false specificity.

## Initial Acceptance Criteria

The classifier pipeline planning work is ready for implementation when:

- this plan is linked from the retrieval improvement index
- an OpenSpec proposal/design/tasks/spec exists
- classifier output includes confidence and reasons
- classifier configuration supports all documented built-in models and subformat families
- classifier eval metrics are defined separately from retrieval metrics
- sample intake rules are documented
- implementation phases can be tested incrementally with saved eval results
