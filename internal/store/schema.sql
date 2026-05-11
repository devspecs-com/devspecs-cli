-- DevSpecs v0.1 schema (version 7)

CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
  id                 TEXT PRIMARY KEY,
  root_path          TEXT NOT NULL,
  git_remote_url     TEXT,
  git_current_branch TEXT,
  last_scan_commit   TEXT,
  last_scan_at       TEXT,
  scanned_by         TEXT,
  created_at         TEXT NOT NULL,
  updated_at         TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
  id                  TEXT PRIMARY KEY,
  repo_id             TEXT,
  short_id            TEXT,
  kind                TEXT NOT NULL,
  subtype             TEXT NOT NULL DEFAULT '',
  title               TEXT NOT NULL,
  status              TEXT NOT NULL DEFAULT 'unknown',
  canonical_source_id TEXT,
  current_revision_id TEXT,
  created_at          TEXT NOT NULL,
  updated_at          TEXT NOT NULL,
  last_observed_at    TEXT NOT NULL,
  authored_at         TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (repo_id) REFERENCES repos(id)
);

CREATE TABLE IF NOT EXISTS artifact_revisions (
  id             TEXT PRIMARY KEY,
  artifact_id    TEXT NOT NULL,
  content_hash   TEXT NOT NULL,
  body           TEXT NOT NULL,
  extracted_json TEXT,
  observed_at    TEXT NOT NULL,
  git_commit     TEXT,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id)
);

CREATE TABLE IF NOT EXISTS sources (
  id              TEXT PRIMARY KEY,
  artifact_id     TEXT NOT NULL,
  repo_id         TEXT,
  source_type     TEXT NOT NULL,
  path            TEXT,
  url             TEXT,
  source_identity TEXT NOT NULL,
  format_profile  TEXT NOT NULL DEFAULT 'generic',
  layout_group    TEXT,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id),
  FOREIGN KEY (repo_id) REFERENCES repos(id)
);

CREATE TABLE IF NOT EXISTS links (
  id          TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL,
  link_type   TEXT NOT NULL,
  target      TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id)
);

CREATE TABLE IF NOT EXISTS artifact_todos (
  id          TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  ordinal     INTEGER NOT NULL,
  text        TEXT NOT NULL,
  done        INTEGER NOT NULL CHECK (done IN (0, 1)),
  source_file TEXT NOT NULL,
  source_line INTEGER NOT NULL,
  created_at  TEXT NOT NULL,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id) REFERENCES artifact_revisions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS artifact_criteria (
  id             TEXT PRIMARY KEY,
  artifact_id    TEXT NOT NULL,
  revision_id    TEXT NOT NULL,
  ordinal        INTEGER NOT NULL,
  text           TEXT NOT NULL,
  done           INTEGER NOT NULL CHECK (done IN (0, 1)),
  source_file    TEXT NOT NULL,
  source_line    INTEGER NOT NULL,
  criteria_kind  TEXT NOT NULL,
  created_at     TEXT NOT NULL,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id) REFERENCES artifact_revisions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS artifact_tags (
  artifact_id TEXT NOT NULL,
  tag         TEXT NOT NULL,
  source      TEXT NOT NULL DEFAULT 'frontmatter',
  created_at  TEXT NOT NULL,
  PRIMARY KEY (artifact_id, tag),
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_todos_artifact ON artifact_todos(artifact_id);
CREATE INDEX IF NOT EXISTS idx_todos_revision ON artifact_todos(revision_id);
CREATE INDEX IF NOT EXISTS idx_criteria_artifact ON artifact_criteria(artifact_id);
CREATE INDEX IF NOT EXISTS idx_criteria_revision ON artifact_criteria(revision_id);
CREATE INDEX IF NOT EXISTS idx_sources_identity ON sources(source_identity);
CREATE INDEX IF NOT EXISTS idx_artifacts_repo ON artifacts(repo_id);
CREATE INDEX IF NOT EXISTS idx_revisions_artifact ON artifact_revisions(artifact_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON artifact_tags(tag);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_short_id ON artifacts(short_id);

CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
  artifact_id UNINDEXED,
  title,
  body,
  source_path,
  tokenize='unicode61'
);
