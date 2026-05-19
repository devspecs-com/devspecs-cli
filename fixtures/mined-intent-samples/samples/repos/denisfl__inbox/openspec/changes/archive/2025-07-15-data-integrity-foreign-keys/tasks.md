## 1. Database Configuration

- [x] 1.1 Add `pragmas: foreign_keys: 1` to `config/database.yml` for development, test, and production environments
- [x] 1.2 Verify FK pragma is active: `ActiveRecord::Base.connection.execute("PRAGMA foreign_keys").first`

## 2. Orphan Cleanup & Foreign Keys Migration

- [x] 2.1 Create migration that deletes orphan `document_links` (where source or target document doesn't exist)
- [x] 2.2 Delete orphan `document_tags`, `task_tags`, `calendar_event_tags` (where document/task/event or tag doesn't exist)
- [x] 2.3 Delete orphan `blocks` (where document doesn't exist)
- [x] 2.4 Add `add_foreign_key :document_links, :documents, column: :source_document_id`
- [x] 2.5 Add `add_foreign_key :document_links, :documents, column: :target_document_id`
- [x] 2.6 Add `add_foreign_key :document_tags, :documents` and `add_foreign_key :document_tags, :tags`
- [x] 2.7 Add `add_foreign_key :task_tags, :tasks` and `add_foreign_key :task_tags, :tags`
- [x] 2.8 Add `add_foreign_key :calendar_event_tags, :calendar_events` and `add_foreign_key :calendar_event_tags, :tags`
- [x] 2.9 Add `add_foreign_key :blocks, :documents`

## 3. Model Associations

- [x] 3.1 Add `dependent: :delete_all` to `Document` for `document_tags`, `outgoing_links`, `incoming_links`
- [x] 3.2 Add `dependent: :destroy` to `Document` for `blocks` and `rich_text_body`
- [x] 3.3 Add `dependent: :delete_all` to `Tag` for `document_tags`, `task_tags`, `calendar_event_tags`
- [x] 3.4 Add `dependent: :delete_all` to `Task` for `task_tags`
- [x] 3.5 Add `dependent: :delete_all` to `CalendarEvent` for `calendar_event_tags`
- [x] 3.6 Verify no existing `dependent:` options conflict with new ones

## 4. Integrity Check Rake Task

- [x] 4.1 Create `lib/tasks/integrity.rake` with `db:integrity_check` task
- [x] 4.2 Check all FK relationships for orphan records (document_links, tags, blocks)
- [x] 4.3 Check ActionText rich_text records for orphan references
- [x] 4.4 Output clear report: table, record id, FK column, missing reference
- [x] 4.5 Exit code 0 on pass, 1 on failure

## 5. Tests

- [x] 5.1 Test FK enforcement: verify inserting a `document_link` with non-existent document_id raises error
- [x] 5.2 Test cascade deletion: deleting a document removes its links, tags, blocks
- [x] 5.3 Test `rake db:integrity_check` with clean DB (passes) and with injected orphan (fails)
