## ADDED Requirements

### Requirement: SQLite foreign key enforcement
The system SHALL enable SQLite foreign key constraints via `PRAGMA foreign_keys = ON` for all database connections.

#### Scenario: FK pragma enabled on connection
- **WHEN** the application establishes a database connection
- **THEN** `PRAGMA foreign_keys` SHALL return `1` (enabled)

### Requirement: Explicit foreign keys in schema
All join tables and FK columns SHALL have explicit `add_foreign_key` declarations in the database schema.

#### Scenario: document_links table
- **WHEN** inspecting the `document_links` table
- **THEN** `source_document_id` and `target_document_id` SHALL have foreign keys referencing `documents(id)`

#### Scenario: document_tags table
- **WHEN** inspecting the `document_tags` table
- **THEN** `document_id` SHALL reference `documents(id)` and `tag_id` SHALL reference `tags(id)`

#### Scenario: task_tags table
- **WHEN** inspecting the `task_tags` table
- **THEN** `task_id` SHALL reference `tasks(id)` and `tag_id` SHALL reference `tags(id)`

#### Scenario: calendar_event_tags table
- **WHEN** inspecting the `calendar_event_tags` table
- **THEN** `calendar_event_id` SHALL reference `calendar_events(id)` and `tag_id` SHALL reference `tags(id)`

### Requirement: Cascade rules on associations
Models SHALL declare `dependent:` options on has_many associations to prevent orphan records.

#### Scenario: Deleting a document
- **WHEN** a document is deleted
- **THEN** its `document_links` (both outgoing and incoming), `document_tags`, `blocks`, and `rich_text_body` SHALL be automatically deleted

#### Scenario: Deleting a tag
- **WHEN** a tag is deleted
- **THEN** its `document_tags`, `task_tags`, and `calendar_event_tags` SHALL be automatically deleted
