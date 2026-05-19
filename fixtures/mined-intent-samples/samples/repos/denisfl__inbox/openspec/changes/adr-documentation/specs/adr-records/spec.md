## ADDED Requirements

### Requirement: ADR directory and template

The project SHALL maintain architecture decision records in `docs/adr/` using a standardized template.

#### Scenario: ADR exists for each key decision

- **GIVEN** the `docs/adr/` directory
- **THEN** it SHALL contain ADRs for: SQLite selection, Parakeet v3 transcription, LLM removal, token-based auth, SolidQueue adoption

#### Scenario: ADR template consistency

- **GIVEN** each ADR file
- **THEN** it SHALL follow MADR format: Title, Status (accepted/deprecated/superseded), Context, Decision, Consequences

#### Scenario: New ADR creation

- **WHEN** a significant architectural decision is made
- **THEN** a new ADR SHALL be created using the template with sequential numbering

### Requirement: Data model documentation

The project SHALL maintain a `docs/data-model.md` documenting the database schema.

#### Scenario: Entity documentation

- **GIVEN** `docs/data-model.md`
- **THEN** it SHALL document all models: Document, Task, CalendarEvent, Tag, and their join tables, with relationships and key fields
