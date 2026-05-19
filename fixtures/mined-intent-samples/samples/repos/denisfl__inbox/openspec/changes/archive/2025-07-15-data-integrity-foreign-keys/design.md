## Context

SQLite supports foreign keys since version 3.6.19, but requires explicit enabling via `PRAGMA foreign_keys = ON`. Rails 6+ can set this via `database.yml`. Currently, FK relationships are implied by column naming conventions but not enforced at the DB level.

Tables with FK columns: `document_links`, `document_tags`, `task_tags`, `calendar_event_tags`, `blocks`, `action_text_rich_texts`.

## Goals / Non-Goals

**Goals:**
- Enable FK enforcement at database level
- Add explicit FK declarations for all existing relationships
- Proper cascade rules on model associations
- Automated integrity check runnable in CI

**Non-Goals:**
- Changing the database engine (staying with SQLite)
- Adding FK constraints for ActionText internal tables (managed by Rails)
- Complex consistency checks beyond FK relationships

## Decisions

### 1. Enable FK pragma via database.yml
**Choice**: Set `pragmas: foreign_keys: 1` in `config/database.yml` for all environments.
**Rationale**: This is the standard Rails way. Applied on every connection, including test.
**Alternative considered**: Initializer with raw SQL — fragile, could miss connections.

### 2. Clean up orphans before adding FK constraints
**Choice**: Migration first checks for and deletes orphan records, then adds foreign keys.
**Rationale**: Adding FK constraints to a table with orphan records will fail in SQLite. Must clean first.

### 3. dependent: :delete_all for join tables, :destroy for rich associations
**Choice**: Use `delete_all` for simple join tables (no callbacks needed), `destroy` for associations that have their own callbacks.
**Rationale**: `delete_all` is faster for join tables. `destroy` ensures callbacks fire for complex objects.

## Risks / Trade-offs

- **[Risk] Orphan records in production** → Migration must handle cleanup. Mitigation: add cleanup step before FK creation, log cleaned records.
- **[Risk] SQLite FK performance** → FK checks add minimal overhead per INSERT/UPDATE. Negligible for our volume.
- **[Trade-off] Cannot add FK to action_text tables** → ActionText manages its own schema. We validate these relationships in the integrity check instead.

## Migration Plan

1. Enable `PRAGMA foreign_keys = ON` in `database.yml`
2. Run cleanup migration (delete orphan records from all FK tables)
3. Add `add_foreign_key` declarations in migration
4. Add/verify `dependent:` options on all models
5. Create `rake db:integrity_check`
6. Add integrity check to CI pipeline

**Rollback**: Remove FK declarations via down migration. Pragma can be removed from `database.yml`.
