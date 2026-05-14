## ADDED Requirements

### Requirement: Adapter classifier pipeline

DevSpecs MUST support deterministic document classification before retrieval consumes artifact type, authority, or lifecycle metadata.

#### Scenario: container document model emits child artifacts

- **GIVEN** a candidate directory matches an OpenSpec change layout
- **AND** the directory contains `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md`
- **WHEN** declarative document models evaluate the candidate
- **THEN** the OpenSpec container model can win with confidence and reasons
- **AND** emits child document candidates for the contained proposal, design, tasks, and spec deltas
- **AND** the child artifacts remain independently retrievable

#### Scenario: high-confidence document model wins

- **GIVEN** a candidate document has strong ADR structure with status, context, decision, and consequences
- **WHEN** declarative document models evaluate the document
- **THEN** the ADR model can win with confidence and reasons
- **AND** the resulting artifact metadata includes decision kind, status, authority, and classifier provenance

#### Scenario: primitive document model emits one artifact

- **GIVEN** a candidate file matches a PRD or ADR document pattern
- **WHEN** the corresponding document model wins
- **THEN** DevSpecs emits one primary artifact for that file
- **AND** does not require a container layout

#### Scenario: ADR subformat evidence is captured

- **GIVEN** a candidate document matches Nygard, MADR, or Y-Statement ADR structure
- **WHEN** the ADR document model evaluates the document
- **THEN** the classifier emits the ADR model as the winning model
- **AND** emits the matching ADR subformat as evidence
- **AND** does not create a separate top-level artifact kind for the subformat

#### Scenario: ambiguous document falls back to generic markdown

- **GIVEN** a candidate document has weak plan-like and PRD-like signals
- **WHEN** no document model wins with sufficient confidence and separation
- **THEN** DevSpecs classifies the document as generic markdown
- **AND** preserves ambiguity and alternative classifier reasons

#### Scenario: configured path acts as a hint

- **GIVEN** a user-configured path suggests a document type
- **AND** the document content strongly contradicts that type
- **WHEN** declarative document models evaluate the candidate
- **THEN** the configured path can influence confidence
- **BUT** it MUST NOT force a clearly wrong high-confidence classification

### Requirement: Classifier explainability

Classifier decisions MUST expose auditable positive and negative reasons.

#### Scenario: negative evidence lowers confidence

- **GIVEN** a markdown file shares terms with implementation plans
- **AND** it contains generated-doc or changelog-only markers
- **WHEN** the plan document model evaluates the file
- **THEN** negative reasons reduce confidence
- **AND** those reasons are available in eval JSON

### Requirement: Classifier configuration

DevSpecs MUST support a versioned classifier configuration structure that covers documented built-in document models and subformat/family models.

#### Scenario: document models are declarative

- **GIVEN** the built-in classifier profile is loaded
- **WHEN** a model contributes confidence
- **THEN** the contribution comes from configured evidence rule IDs, predicates, weights, and reasons
- **AND** the Go implementation acts as a generic evaluator rather than a hard-coded classifier per document type

#### Scenario: built-in defaults cover documented models

- **GIVEN** no repo classifier overrides are configured
- **WHEN** the classifier pipeline is initialized
- **THEN** DevSpecs loads a built-in classifier profile
- **AND** the profile includes OpenSpec, ADR, RFC/proposal, PRD, plan, agent-note, and generic markdown models
- **AND** the profile records whether each model supports container scope, document scope, or both
- **AND** the ADR model includes Nygard, MADR, and Y-Statement subformat evidence

#### Scenario: repo override adds hints without forcing classification

- **GIVEN** a repo classifier override adds local RFC path hints
- **AND** a candidate in that path lacks RFC/proposal document features
- **WHEN** the classifier resolver evaluates the candidate
- **THEN** the path hint can influence confidence
- **BUT** it MUST NOT force a high-confidence RFC classification by itself

#### Scenario: local model inherits a built-in model

- **GIVEN** a repo defines a local document model based on RFC/proposal
- **WHEN** the local model matches configured headings and terms
- **THEN** DevSpecs can emit the local model as classifier evidence
- **AND** preserves the built-in base model for retrieval compatibility

### Requirement: Conservative RFC and PRD families

DevSpecs MUST treat RFC and PRD document models as section-pattern families unless real samples justify named subformats.

#### Scenario: RFC section pattern

- **GIVEN** a candidate document has engineering proposal sections such as summary, motivation, proposal, alternatives, risks, rollout, and open questions
- **WHEN** declarative document models evaluate the document
- **THEN** an RFC/proposal document model can classify it with confidence and reasons
- **AND** the classifier records the matched section family

#### Scenario: PRD product-intent pattern

- **GIVEN** a candidate document has product sections such as goals, non-goals, users, requirements, user stories, acceptance criteria, launch, and success metrics
- **WHEN** declarative document models evaluate the document
- **THEN** the PRD document model can classify it with confidence and reasons
- **AND** DevSpecs does not require a named PRD subformat

### Requirement: Classifier eval metrics

DevSpecs MUST measure classifier quality separately from retrieval quality.

#### Scenario: classifier eval reports confusion

- **GIVEN** classifier golden cases define expected document models
- **WHEN** eval runs classifier scoring
- **THEN** the output reports model accuracy, false positives, false negatives, ambiguity rate, generic fallback rate, reject rate, and top confusion pairs

#### Scenario: classifier eval remains deterministic

- **GIVEN** a fixture and classifier golden cases
- **WHEN** classifier eval runs repeatedly
- **THEN** results are deterministic
- **AND** no network or model calls are required
