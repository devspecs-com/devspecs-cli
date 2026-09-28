package hubstore

const applicationID = 0x44534842 // DSHB
const schemaVersion = 7

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

// v4 owns durable consumers and cursors. Subscription definitions are immutable;
// changing filters requires a new subscription. No retention deletion is added.
const schemaV4 = `
CREATE TABLE hub_meta_v4 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 4),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v4 SELECT singleton, db_id, 4, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v4 RENAME TO hub_meta;
CREATE TABLE pull_token_secret (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  secret BLOB NOT NULL CHECK (length(secret) = 32)
);
CREATE TABLE consumers (
  consumer_id TEXT PRIMARY KEY,
  enrolled_at INTEGER NOT NULL
);
CREATE TABLE subscriptions (
  subscription_id TEXT PRIMARY KEY,
  consumer_id TEXT NOT NULL REFERENCES consumers(consumer_id),
  scope_id TEXT NOT NULL REFERENCES repo_scopes(scope_id),
  kinds TEXT NOT NULL,
  event_types TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  acknowledged_sequence INTEGER NOT NULL CHECK (acknowledged_sequence >= 0),
  removed_at INTEGER,
  CHECK (removed_at IS NULL OR removed_at >= created_at)
);
CREATE INDEX subscriptions_by_consumer ON subscriptions(consumer_id,scope_id,created_at,subscription_id);
CREATE TABLE subscription_topics (
  subscription_id TEXT NOT NULL REFERENCES subscriptions(subscription_id),
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  PRIMARY KEY(subscription_id,topic_id),
  FOREIGN KEY(scope_id,topic_id) REFERENCES topics(scope_id,topic_id)
);
CREATE INDEX subscription_topics_by_topic ON subscription_topics(scope_id,topic_id);
CREATE TRIGGER subscription_definition_immutable BEFORE UPDATE OF subscription_id,consumer_id,scope_id,kinds,event_types,created_at ON subscriptions
BEGIN SELECT RAISE(ABORT, 'subscription definition is immutable'); END;
CREATE TRIGGER subscriptions_no_delete BEFORE DELETE ON subscriptions BEGIN SELECT RAISE(ABORT, 'subscription is retained'); END;
CREATE TRIGGER subscription_topics_no_update BEFORE UPDATE ON subscription_topics BEGIN SELECT RAISE(ABORT, 'subscription topic is immutable'); END;
CREATE TRIGGER subscription_topics_no_delete BEFORE DELETE ON subscription_topics BEGIN SELECT RAISE(ABORT, 'subscription topic is retained'); END;
`

// v5 permits deletion only inside an explicit retention transaction. Durable
// ranges and keyed tombstones survive after payload rows are reclaimed.
const schemaV5 = `
CREATE TABLE hub_meta_v5 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 5),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v5 SELECT singleton, db_id, 5, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v5 RENAME TO hub_meta;
CREATE TABLE retention_permission (singleton INTEGER PRIMARY KEY CHECK (singleton=1), enabled INTEGER NOT NULL CHECK (enabled IN (0,1)));
INSERT INTO retention_permission VALUES (1,0);
CREATE TABLE pruned_ranges (
  scope_id TEXT NOT NULL REFERENCES repo_scopes(scope_id),
  from_sequence INTEGER NOT NULL,
  to_sequence INTEGER NOT NULL,
  prune_id TEXT NOT NULL,
  PRIMARY KEY(scope_id,from_sequence),
  CHECK(from_sequence > 0 AND to_sequence >= from_sequence)
);
CREATE INDEX pruned_ranges_end ON pruned_ranges(scope_id,to_sequence);
CREATE TABLE pruned_idempotency (
  scope_id TEXT NOT NULL,
  topic_id TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  key TEXT NOT NULL,
  prune_id TEXT NOT NULL,
  PRIMARY KEY(scope_id,topic_id,actor_id,key),
  FOREIGN KEY(scope_id,topic_id) REFERENCES topics(scope_id,topic_id)
);
CREATE TABLE prune_audit (
  prune_id TEXT PRIMARY KEY,
  cutoff_at INTEGER NOT NULL,
  pruned_at INTEGER NOT NULL,
  backup_path TEXT NOT NULL,
  backup_available INTEGER NOT NULL DEFAULT 1 CHECK (backup_available IN (0,1)),
  entry_count INTEGER NOT NULL,
  payload_bytes INTEGER NOT NULL,
  retention_epoch INTEGER NOT NULL
);
DROP TRIGGER publications_no_delete;
DROP TRIGGER message_revisions_no_delete;
DROP TRIGGER message_pin_audit_no_delete;
DROP TRIGGER publication_idempotency_no_delete;
DROP TRIGGER event_entries_no_delete;
CREATE TRIGGER publications_no_delete BEFORE DELETE ON publications WHEN (SELECT enabled FROM retention_permission WHERE singleton=1) != 1 BEGIN SELECT RAISE(ABORT, 'publication is retained'); END;
CREATE TRIGGER message_revisions_no_delete BEFORE DELETE ON message_revisions WHEN (SELECT enabled FROM retention_permission WHERE singleton=1) != 1 BEGIN SELECT RAISE(ABORT, 'revision is retained'); END;
CREATE TRIGGER message_pin_audit_no_delete BEFORE DELETE ON message_pin_audit WHEN (SELECT enabled FROM retention_permission WHERE singleton=1) != 1 BEGIN SELECT RAISE(ABORT, 'pin audit is retained'); END;
CREATE TRIGGER publication_idempotency_no_delete BEFORE DELETE ON publication_idempotency WHEN (SELECT enabled FROM retention_permission WHERE singleton=1) != 1 BEGIN SELECT RAISE(ABORT, 'idempotency is retained'); END;
CREATE TRIGGER event_entries_no_delete BEFORE DELETE ON event_entries WHEN (SELECT enabled FROM retention_permission WHERE singleton=1) != 1 BEGIN SELECT RAISE(ABORT, 'event is retained'); END;
`

// Votes are mutable advisory state for one actor and one exact revision.
// Retention removes them explicitly before deleting their revision.
const schemaV6 = `
CREATE TABLE hub_meta_v6 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 6),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v6 SELECT singleton, db_id, 6, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v6 RENAME TO hub_meta;
CREATE TABLE message_votes (
  message_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  actor_id TEXT NOT NULL REFERENCES actors(actor_id),
  value INTEGER NOT NULL CHECK (value IN (-1,1)),
  PRIMARY KEY (message_id,revision,actor_id),
  FOREIGN KEY (message_id,revision) REFERENCES message_revisions(message_id,revision) ON DELETE CASCADE
);
`

// v7 adds one home-global scope without borrowing a Git repository binding.
// Child foreign keys keep referencing repo_scopes after the table is replaced.
const schemaV7 = `
CREATE TABLE hub_meta_v7 (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  db_id TEXT NOT NULL,
  format_version INTEGER NOT NULL CHECK (format_version = 7),
  retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch >= 0)
);
INSERT INTO hub_meta_v7 SELECT singleton, db_id, 7, retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v7 RENAME TO hub_meta;
CREATE TABLE repo_scopes_v7 (
  scope_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('git', 'global')),
  enrolled_at INTEGER NOT NULL,
  low_water_sequence INTEGER NOT NULL DEFAULT 0 CHECK (low_water_sequence >= 0)
);
INSERT INTO repo_scopes_v7 SELECT scope_id, kind, enrolled_at, low_water_sequence FROM repo_scopes;
DROP TABLE repo_scopes;
ALTER TABLE repo_scopes_v7 RENAME TO repo_scopes;
CREATE UNIQUE INDEX repo_scopes_one_global ON repo_scopes(kind) WHERE kind='global';
`
