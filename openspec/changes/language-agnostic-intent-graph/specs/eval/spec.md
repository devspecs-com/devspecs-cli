## ADDED Requirements

### Requirement: Multi-stack retrieval evaluation

The eval fixture SHOULD include source/context cases that are not limited to TypeScript.

#### Scenario: non-TypeScript source candidate

- **GIVEN** a fixture contains a relevant source or context file outside TypeScript
- **WHEN** a query references a path, identifier, or source/code entity from that file
- **THEN** the eval can mark that file as expected relevant
- **AND** retrieval quality is measured with the same recall, precision, and sufficiency metrics

### Requirement: Graph-aware explanation coverage

Eval JSON SHOULD expose deterministic reasons for graph-aware retrieval decisions.

#### Scenario: artifact included through an edge

- **GIVEN** an artifact is included because it is a companion, constraint, supersession, or background artifact
- **WHEN** eval JSON is emitted
- **THEN** the artifact reason identifies the edge or relationship type

### Requirement: Language overfit guardrail

Eval SHOULD make language-specific regressions visible before benchmark claims.

#### Scenario: retrieval only improves TypeScript cases

- **GIVEN** retrieval changes improve TypeScript fixture cases
- **AND** non-TypeScript source/context cases do not improve or regress
- **WHEN** benchmark summaries are reviewed
- **THEN** the eval output includes enough per-case detail to show the imbalance
