## ADDED Requirements

### Requirement: Language-agnostic retrieval model

Retrieval MUST model artifacts, sections, entities, and edges without assuming a specific programming language or stack.

#### Scenario: repo has no supported AST parser

- **GIVEN** a repo contains markdown intent artifacts and source files in an unsupported language
- **WHEN** DevSpecs builds retrieval candidates
- **THEN** it can still extract paths, headings, slugs, lifecycle labels, and identifier-shaped terms
- **AND** retrieval does not require language-specific AST support

#### Scenario: source symbols are extracted

- **GIVEN** a source adapter extracts a function, class, type, resource, migration object, or exported symbol
- **WHEN** the symbol is added to the retrieval index
- **THEN** it is represented as a generic source/code entity
- **AND** the retrieval model does not expose TypeScript-specific concepts as core primitives

### Requirement: Section-aware intent retrieval

Retrieval SHOULD score meaningful artifact sections before deciding which artifacts to include.

#### Scenario: implementation task section matches query

- **GIVEN** a plan contains generic background text and a specific implementation checklist
- **WHEN** the query asks for implementation context
- **THEN** retrieval can score the checklist section higher than generic background
- **AND** the selected artifact reason identifies the section role

#### Scenario: stale section matches stale-history query

- **GIVEN** an artifact has a stale or superseded section
- **WHEN** the query asks for stale or superseded history
- **THEN** retrieval can prefer that section over active implementation sections

### Requirement: Local alias expansion

Retrieval SHOULD expand query terms using deterministic local evidence rather than global synonym assumptions.

#### Scenario: identifier variant alias

- **GIVEN** local artifacts contain `webhook_replay_protection`
- **WHEN** a query says `webhook replay protection`
- **THEN** retrieval can match the identifier variant
- **AND** the reason reports identifier normalization or alias expansion

#### Scenario: unsupported global synonym

- **GIVEN** two words are generic synonyms but never linked by local evidence
- **WHEN** retrieval expands query terms
- **THEN** it does not add the synonym solely from a global thesaurus

### Requirement: Graph edge retrieval

Retrieval SHOULD use deterministic artifact edges for high-confidence companion and constraint relationships.

#### Scenario: OpenSpec companion expansion

- **GIVEN** an implementation query strongly matches an OpenSpec change
- **WHEN** the change has proposal, design, tasks, and spec delta artifacts
- **THEN** retrieval can include companion artifacts through `openspec_companion` edges
- **AND** the output explains the edge reason

#### Scenario: lifecycle edge prevents stale pollution

- **GIVEN** a stale scratch plan and an active design share generic terms
- **WHEN** the query asks for current implementation context
- **THEN** lifecycle edges or labels can downrank the stale scratch plan
