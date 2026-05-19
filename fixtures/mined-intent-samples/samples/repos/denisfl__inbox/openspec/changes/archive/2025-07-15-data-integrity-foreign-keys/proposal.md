## Why

As the project grows with new features (document_links, embeddings, statuses), the number of related records increases. There are no explicit foreign keys in the database, and no automated integrity checks. Deleting or modifying a document could leave orphan records in `document_links`, `action_text_rich_texts`, or future tables.

## What Changes

- Enable SQLite foreign key enforcement (`PRAGMA foreign_keys = ON`) in `database.yml`
- Add explicit `add_foreign_key` declarations for all existing FK relationships
- Add `dependent: :destroy` / `dependent: :delete_all` to model associations where missing
- New `rake db:integrity_check` task that validates referential integrity
- Run integrity check in CI

## Capabilities

### New Capabilities
- `foreign-key-enforcement`: SQLite FK pragma enabled, explicit foreign keys in schema, cascade rules on associations
- `integrity-check-task`: Rake task that detects orphan records and inconsistencies

### Modified Capabilities
<!-- No existing spec-level capability changes -->

## Impact

- **Config**: `config/database.yml` (add `variables: foreign_keys: "ON"`)
- **Migrations**: New migration adding `add_foreign_key` for existing tables (`document_links`, `document_tags`, `task_tags`, `calendar_event_tags`, `blocks`)
- **Models**: Add/verify `dependent:` options on `Document`, `Tag`, `Task`, `CalendarEvent` associations
- **New files**: `lib/tasks/integrity.rake`
- **Risk**: Enabling FK enforcement may surface existing orphan records that need cleanup before migration
