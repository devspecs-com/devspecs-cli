## ADDED Requirements

### Requirement: Identifier-aware retrieval

Retrieval MUST preserve and match identifier-shaped terms in paths and content.

#### Scenario: snake_case query

- **GIVEN** a query contains `authorization_details`
- **WHEN** relevant source and planning files contain that exact identifier
- **THEN** retrieval considers the exact identifier match
- **AND** does not require splitting the identifier into unrelated terms only

#### Scenario: dated plan slug query

- **GIVEN** a query contains `pnpm`
- **AND** a dated plan filename contains `pnpm`
- **WHEN** retrieval runs
- **THEN** the dated plan can be retrieved from its path match

### Requirement: OpenSpec bundle retrieval

Retrieval MUST support OpenSpec implementation context as a related file bundle.

#### Scenario: implementation query matches OpenSpec change

- **GIVEN** an OpenSpec change has `proposal.md`, `design.md`, `tasks.md`, and spec deltas
- **WHEN** a query asks for implementation or agent context for that change
- **THEN** retrieval considers the related design, tasks, and spec delta files

### Requirement: Authority and lifecycle-aware ranking

Retrieval SHOULD prefer authoritative current artifacts over stale or low-authority artifacts when query intent supports that.

#### Scenario: implementation query matches stale scratch

- **GIVEN** a stale scratch file shares many query terms
- **AND** an active ADR or OpenSpec design also matches
- **WHEN** the query asks for implementation context
- **THEN** the active authoritative artifact ranks ahead of stale scratch

#### Scenario: stale-history query

- **GIVEN** a query asks for stale or superseded local entitlement caching
- **WHEN** a superseded ADR explains the abandoned design
- **THEN** retrieval can include the superseded ADR
- **AND** should avoid presenting active OpenSpec implementation as the answer to stale-history intent

