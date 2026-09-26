package hubstore

const applicationID = 0x44534842 // DSHB
const schemaVersion = 3

// v1 is deliberately hub-only. A later migration must advance user_version and
// insert its digest in the same transaction.
const schemaV1 = `
CREATE TABLE hub_meta (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 1),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_at INTEGER NOT NULL
);
CREATE TABLE actors (
  actor_id TEXT PRIMARY KEY,
  enrolled_at INTEGER NOT NULL
);
CREATE TABLE repo_scopes (
  scope_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind = 'git'),
  enrolled_at INTEGER NOT NULL,
  low_water_sequence INTEGER NOT NULL DEFAULT 0 CHECK (low_water_sequence >= 0)
);
CREATE TABLE scope_bindings (
  scope_id TEXT PRIMARY KEY REFERENCES repo_scopes(scope_id),
  common_dir TEXT NOT NULL UNIQUE,
  marker TEXT NOT NULL,
  file_identity TEXT NOT NULL
);
CREATE INDEX scope_bindings_marker ON scope_bindings(marker);
CREATE TABLE topics (
  topic_id TEXT PRIMARY KEY,
  scope_id TEXT NOT NULL REFERENCES repo_scopes(scope_id),
  key TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  owner_actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  created_at INTEGER NOT NULL,
  expires_at INTEGER,
  archived_at INTEGER,
  archive_actor_id TEXT REFERENCES actors(actor_id),
  archive_reason TEXT,
  policy_generation INTEGER NOT NULL CHECK (policy_generation > 0),
  UNIQUE(scope_id, key),
  UNIQUE(scope_id, topic_id),
  CHECK ((archived_at IS NULL AND archive_actor_id IS NULL AND archive_reason IS NULL) OR
         (archived_at IS NOT NULL AND archive_actor_id IS NOT NULL AND archive_reason IS NOT NULL))
);
CREATE INDEX topics_discovery ON topics(scope_id, created_at DESC, topic_id DESC);
CREATE TABLE topic_roles (
  topic_id TEXT NOT NULL REFERENCES topics(topic_id),
  actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  role TEXT NOT NULL CHECK (role = 'maintainer'),
  PRIMARY KEY(topic_id, actor_id, role)
);
CREATE TABLE topic_audit (
  audit_id TEXT PRIMARY KEY,
  topic_id TEXT NOT NULL REFERENCES topics(topic_id),
  actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  action TEXT NOT NULL,
  at INTEGER NOT NULL,
  old_value TEXT NOT NULL,
  new_value TEXT NOT NULL
);
CREATE INDEX topic_audit_by_topic ON topic_audit(topic_id, at, audit_id);
CREATE TRIGGER topic_identity_immutable BEFORE UPDATE OF topic_id, scope_id, key ON topics
BEGIN SELECT RAISE(ABORT, 'topic identity is immutable'); END;
CREATE TRIGGER topic_no_delete BEFORE DELETE ON topics
BEGIN SELECT RAISE(ABORT, 'topics are retained'); END;
CREATE TRIGGER topic_audit_no_update BEFORE UPDATE ON topic_audit
BEGIN SELECT RAISE(ABORT, 'audit is immutable'); END;
CREATE TRIGGER topic_audit_no_delete BEFORE DELETE ON topic_audit
BEGIN SELECT RAISE(ABORT, 'audit is retained'); END;
`

// v2 adds the append-only publication stream. Sequence counters and low-water
// markers are never reset by expiry or archive; physical pruning is not here.
const schemaV2 = `
CREATE TABLE hub_meta_v2 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 2),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v2 SELECT singleton, db_id, 2, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v2 RENAME TO hub_meta;
CREATE TABLE scope_counters (
  scope_id TEXT PRIMARY KEY REFERENCES repo_scopes(scope_id),
  next_sequence INTEGER NOT NULL DEFAULT 1 CHECK (next_sequence > 0)
);
INSERT INTO scope_counters (scope_id) SELECT scope_id FROM repo_scopes;
CREATE TABLE publications (
  entry_id TEXT PRIMARY KEY,
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  kind TEXT NOT NULL CHECK (kind IN ('message','event')),
  sequence INTEGER NOT NULL CHECK (sequence > 0),
  committed_at INTEGER NOT NULL,
  occurred_at INTEGER,
  expires_at INTEGER,
  source_refs TEXT NOT NULL,
  correlation_refs TEXT NOT NULL,
  idempotency_key TEXT,
  FOREIGN KEY (scope_id, topic_id) REFERENCES topics(scope_id, topic_id),
  UNIQUE(scope_id, sequence),
  UNIQUE(entry_id, scope_id, topic_id),
  UNIQUE(entry_id, scope_id, topic_id, actor_id),
  UNIQUE(entry_id, scope_id, topic_id, expires_at)
);
CREATE INDEX publications_topic_sequence ON publications(scope_id, topic_id, sequence);
CREATE TABLE message_heads (
  message_id TEXT PRIMARY KEY,
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  author_actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  current_revision INTEGER NOT NULL CHECK (current_revision > 0),
  current_entry_id TEXT NOT NULL REFERENCES publications(entry_id),
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0,1)),
  FOREIGN KEY (scope_id, topic_id) REFERENCES topics(scope_id, topic_id),
  UNIQUE(message_id, scope_id, topic_id)
);
CREATE TABLE message_revisions (
  message_id TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK (revision > 0),
  entry_id TEXT NOT NULL UNIQUE,
  text TEXT NOT NULL,
  expires_at INTEGER,
  PRIMARY KEY (message_id, revision),
  FOREIGN KEY (message_id, scope_id, topic_id) REFERENCES message_heads(message_id, scope_id, topic_id),
  FOREIGN KEY (entry_id, scope_id, topic_id) REFERENCES publications(entry_id, scope_id, topic_id),
  FOREIGN KEY (entry_id, scope_id, topic_id, expires_at) REFERENCES publications(entry_id, scope_id, topic_id, expires_at)
);
CREATE TABLE message_pin_audit (
  pin_id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL REFERENCES message_heads(message_id),
  revision INTEGER NOT NULL,
  actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  pinned INTEGER NOT NULL CHECK (pinned IN (0,1)),
  at INTEGER NOT NULL,
  FOREIGN KEY (message_id, revision) REFERENCES message_revisions(message_id, revision)
);
CREATE TABLE publication_idempotency (
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  key TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  entry_id TEXT NOT NULL REFERENCES publications(entry_id),
  PRIMARY KEY(scope_id, topic_id, actor_id, key),
  FOREIGN KEY (scope_id, topic_id) REFERENCES topics(scope_id, topic_id)
);
CREATE TRIGGER publications_no_update BEFORE UPDATE ON publications BEGIN SELECT RAISE(ABORT, 'publication is immutable'); END;
CREATE TRIGGER publications_no_delete BEFORE DELETE ON publications BEGIN SELECT RAISE(ABORT, 'publication is retained'); END;
CREATE TRIGGER message_revisions_no_update BEFORE UPDATE ON message_revisions BEGIN SELECT RAISE(ABORT, 'revision is immutable'); END;
CREATE TRIGGER message_revisions_no_delete BEFORE DELETE ON message_revisions BEGIN SELECT RAISE(ABORT, 'revision is retained'); END;
CREATE TRIGGER message_pin_audit_no_update BEFORE UPDATE ON message_pin_audit BEGIN SELECT RAISE(ABORT, 'pin audit is immutable'); END;
CREATE TRIGGER message_pin_audit_no_delete BEFORE DELETE ON message_pin_audit BEGIN SELECT RAISE(ABORT, 'pin audit is retained'); END;
CREATE TRIGGER publication_idempotency_no_update BEFORE UPDATE ON publication_idempotency BEGIN SELECT RAISE(ABORT, 'idempotency is immutable'); END;
CREATE TRIGGER publication_idempotency_no_delete BEFORE DELETE ON publication_idempotency BEGIN SELECT RAISE(ABORT, 'idempotency is retained'); END;
`

const schemaV3 = `
CREATE TABLE hub_meta_v3 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 3),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v3 SELECT singleton, db_id, 3, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v3 RENAME TO hub_meta;
CREATE TABLE event_schemas (
  topic_id TEXT NOT NULL REFERENCES topics(topic_id),
  type_key TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version > 0),
  dialect TEXT NOT NULL,
  schema_json TEXT NOT NULL,
  schema_sha256 TEXT NOT NULL,
  registered_by TEXT NOT NULL REFERENCES actors(actor_id),
  registered_at INTEGER NOT NULL,
  retired_by TEXT REFERENCES actors(actor_id),
  retired_at INTEGER,
  PRIMARY KEY(topic_id,type_key,version),
  CHECK ((retired_by IS NULL) = (retired_at IS NULL))
);
CREATE TABLE event_entries (
  entry_id TEXT PRIMARY KEY REFERENCES publications(entry_id),
  topic_id TEXT NOT NULL,
  type_key TEXT NOT NULL,
  version INTEGER NOT NULL,
  schema_sha256 TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  corrects_entry_id TEXT REFERENCES event_entries(entry_id),
  FOREIGN KEY(topic_id,type_key,version) REFERENCES event_schemas(topic_id,type_key,version)
);
CREATE INDEX event_entries_topic ON event_entries(topic_id,type_key,version);
CREATE TRIGGER event_schemas_no_delete BEFORE DELETE ON event_schemas BEGIN SELECT RAISE(ABORT, 'schema is retained'); END;
CREATE TRIGGER event_schemas_immutable BEFORE UPDATE OF topic_id,type_key,version,dialect,schema_json,schema_sha256,registered_by,registered_at ON event_schemas BEGIN SELECT RAISE(ABORT, 'schema is immutable'); END;
CREATE TRIGGER event_schemas_retirement_immutable BEFORE UPDATE OF retired_by,retired_at ON event_schemas WHEN OLD.retired_at IS NOT NULL OR NEW.retired_at IS NULL OR NEW.retired_by IS NULL BEGIN SELECT RAISE(ABORT, 'retirement is immutable'); END;
CREATE TRIGGER event_entries_no_update BEFORE UPDATE ON event_entries BEGIN SELECT RAISE(ABORT, 'event is immutable'); END;
CREATE TRIGGER event_entries_no_delete BEFORE DELETE ON event_entries BEGIN SELECT RAISE(ABORT, 'event is retained'); END;
`
