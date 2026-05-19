## ADDED Requirements

### Requirement: Integrity check rake task
The system SHALL provide `rake db:integrity_check` that validates referential integrity across all tables.

#### Scenario: No orphan records
- **WHEN** `rake db:integrity_check` runs and all records are valid
- **THEN** the task SHALL output "Integrity check passed" and exit with code 0

#### Scenario: Orphan records found
- **WHEN** `rake db:integrity_check` runs and finds `document_links` referencing deleted documents
- **THEN** the task SHALL output each orphan record (table, id, FK column, missing reference) and exit with code 1

#### Scenario: Inconsistent data found
- **WHEN** `rake db:integrity_check` finds documents with `embedding_generated_at` but no `embedding` (or vice versa), if those columns exist
- **THEN** the task SHALL report the inconsistency with document IDs
