package hubstore

const applicationID = 0x44534842 // DSHB
const schemaVersion = 1

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
