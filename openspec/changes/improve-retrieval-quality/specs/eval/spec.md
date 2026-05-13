## ADDED Requirements

### Requirement: Importance-weighted relevance

The eval harness MUST support expected relevant artifacts with importance labels.

#### Scenario: case marks must-have artifact

- **GIVEN** a case defines an expected relevant artifact with `importance: must`
- **WHEN** the eval runs
- **THEN** the result reports whether that must-have artifact was retrieved
- **AND** aggregate output includes must-have recall

#### Scenario: legacy string expected relevant entries

- **GIVEN** a case defines expected relevant artifacts as plain path strings
- **WHEN** the eval runs
- **THEN** those entries remain valid
- **AND** they default to a documented importance level

### Requirement: Deterministic context sufficiency

The eval harness MUST support deterministic sufficiency checks without model calls.

#### Scenario: context contains required terms and artifacts

- **GIVEN** a case defines `must_contain_terms` and `must_contain_artifacts`
- **WHEN** the retrieved context includes all required terms and artifacts
- **THEN** the case sufficiency check passes

#### Scenario: context includes forbidden artifact

- **GIVEN** a case defines `must_not_contain_artifacts`
- **WHEN** the retrieved context includes one of those artifacts
- **THEN** the case sufficiency check fails
- **AND** the failure lists the offending artifact

### Requirement: Pareto summary

The eval output MUST summarize token reduction together with retrieval quality.

#### Scenario: eval prints summary

- **WHEN** the eval completes
- **THEN** the summary includes token reduction, overall recall, must-have recall, precision, and sufficiency pass rate

