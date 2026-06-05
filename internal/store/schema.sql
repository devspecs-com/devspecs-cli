-- DevSpecs v0.1 schema (version 11)

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
  section_id  TEXT NOT NULL DEFAULT '',
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
  section_id      TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS artifact_sections (
  id             TEXT PRIMARY KEY,
  artifact_id    TEXT NOT NULL,
  revision_id    TEXT NOT NULL,
  source_path    TEXT NOT NULL,
  heading_path   TEXT NOT NULL,
  heading_depth  INTEGER NOT NULL,
  start_line     INTEGER NOT NULL,
  end_line       INTEGER NOT NULL,
  title          TEXT NOT NULL,
  body           TEXT NOT NULL,
  token_estimate INTEGER NOT NULL,
  section_kind   TEXT NOT NULL DEFAULT '',
  metadata_json  TEXT NOT NULL DEFAULT '{}',
  created_at     TEXT NOT NULL,
  FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
  FOREIGN KEY (revision_id) REFERENCES artifact_revisions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS concepts (
  id                         TEXT PRIMARY KEY,
  repo_id                    TEXT NOT NULL,
  canonical                  TEXT NOT NULL,
  kind                       TEXT NOT NULL,
  forms_json                 TEXT NOT NULL DEFAULT '[]',
  document_frequency         INTEGER NOT NULL DEFAULT 0,
  inverse_document_frequency REAL NOT NULL DEFAULT 0,
  created_at                 TEXT NOT NULL,
  updated_at                 TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS concept_mentions (
  id            TEXT PRIMARY KEY,
  concept_id    TEXT NOT NULL,
  artifact_id   TEXT NOT NULL,
  section_id    TEXT NOT NULL DEFAULT '',
  field         TEXT NOT NULL,
  weight        REAL NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}',
  created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifact_edges (
  id              TEXT PRIMARY KEY,
  repo_id         TEXT NOT NULL,
  src_artifact_id TEXT NOT NULL,
  dst_artifact_id TEXT NOT NULL,
  edge_type       TEXT NOT NULL,
  weight          REAL NOT NULL,
  confidence      REAL NOT NULL,
  evidence_count  INTEGER NOT NULL DEFAULT 1,
  freshness       TEXT NOT NULL DEFAULT '',
  source_signal   TEXT NOT NULL,
  explanation     TEXT NOT NULL DEFAULT '',
  metadata_json   TEXT NOT NULL DEFAULT '{}',
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS git_commits (
  repo_id       TEXT NOT NULL,
  sha           TEXT NOT NULL,
  branch        TEXT NOT NULL DEFAULT '',
  author_name   TEXT NOT NULL DEFAULT '',
  author_email  TEXT NOT NULL DEFAULT '',
  message       TEXT NOT NULL,
  body_preview  TEXT NOT NULL DEFAULT '',
  committed_at  TEXT NOT NULL,
  files_changed INTEGER NOT NULL DEFAULT 0,
  is_merge      INTEGER NOT NULL DEFAULT 0,
  history_shape TEXT NOT NULL DEFAULT '',
  indexed_at    TEXT NOT NULL,
  PRIMARY KEY (repo_id, sha)
);

CREATE TABLE IF NOT EXISTS git_commit_files (
  repo_id     TEXT NOT NULL,
  commit_sha  TEXT NOT NULL,
  file_path   TEXT NOT NULL,
  change_type TEXT NOT NULL DEFAULT '',
  old_path    TEXT NOT NULL DEFAULT '',
  indexed_at  TEXT NOT NULL,
  PRIMARY KEY (repo_id, commit_sha, file_path)
);

CREATE TABLE IF NOT EXISTS source_manifest (
  file_id           TEXT PRIMARY KEY,
  repo_id           TEXT NOT NULL,
  path              TEXT NOT NULL,
  content_hash      TEXT NOT NULL,
  size_bytes        INTEGER NOT NULL,
  language          TEXT NOT NULL,
  source_root       TEXT NOT NULL,
  source_root_kind  TEXT NOT NULL,
  source_role       TEXT NOT NULL,
  first_party_score REAL NOT NULL DEFAULT 0,
  ignored_reason    TEXT NOT NULL DEFAULT '',
  indexed_at        TEXT NOT NULL,
  UNIQUE(repo_id, path),
  FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_manifest_symbols (
  file_id TEXT NOT NULL,
  symbol  TEXT NOT NULL,
  kind    TEXT NOT NULL DEFAULT '',
  line    INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (file_id) REFERENCES source_manifest(file_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_manifest_tests (
  file_id   TEXT NOT NULL,
  test_name TEXT NOT NULL,
  parent    TEXT NOT NULL DEFAULT '',
  line      INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (file_id) REFERENCES source_manifest(file_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS source_manifest_imports (
  file_id    TEXT NOT NULL,
  import_ref TEXT NOT NULL,
  line       INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (file_id) REFERENCES source_manifest(file_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_todos_artifact ON artifact_todos(artifact_id);
CREATE INDEX IF NOT EXISTS idx_todos_revision ON artifact_todos(revision_id);
CREATE INDEX IF NOT EXISTS idx_todos_section ON artifact_todos(section_id);
CREATE INDEX IF NOT EXISTS idx_criteria_artifact ON artifact_criteria(artifact_id);
CREATE INDEX IF NOT EXISTS idx_criteria_revision ON artifact_criteria(revision_id);
CREATE INDEX IF NOT EXISTS idx_criteria_section ON artifact_criteria(section_id);
CREATE INDEX IF NOT EXISTS idx_sources_identity ON sources(source_identity);
CREATE INDEX IF NOT EXISTS idx_artifacts_repo ON artifacts(repo_id);
CREATE INDEX IF NOT EXISTS idx_revisions_artifact ON artifact_revisions(artifact_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON artifact_tags(tag);
CREATE INDEX IF NOT EXISTS idx_sections_artifact ON artifact_sections(artifact_id);
CREATE INDEX IF NOT EXISTS idx_sections_revision ON artifact_sections(revision_id);
CREATE INDEX IF NOT EXISTS idx_sections_source_path ON artifact_sections(source_path);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_short_id ON artifacts(short_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_concepts_repo_kind_canonical ON concepts(repo_id, kind, canonical);
CREATE INDEX IF NOT EXISTS idx_concept_mentions_concept ON concept_mentions(concept_id);
CREATE INDEX IF NOT EXISTS idx_concept_mentions_artifact ON concept_mentions(artifact_id);
CREATE INDEX IF NOT EXISTS idx_concept_mentions_artifact_section ON concept_mentions(artifact_id, section_id);
CREATE INDEX IF NOT EXISTS idx_artifact_edges_src ON artifact_edges(repo_id, src_artifact_id);
CREATE INDEX IF NOT EXISTS idx_artifact_edges_dst ON artifact_edges(repo_id, dst_artifact_id);
CREATE INDEX IF NOT EXISTS idx_artifact_edges_type ON artifact_edges(repo_id, edge_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_edges_identity ON artifact_edges(repo_id, src_artifact_id, dst_artifact_id, edge_type, source_signal);
CREATE INDEX IF NOT EXISTS idx_git_commits_repo_committed ON git_commits(repo_id, committed_at);
CREATE INDEX IF NOT EXISTS idx_git_commit_files_repo_file ON git_commit_files(repo_id, file_path);
CREATE INDEX IF NOT EXISTS idx_git_commit_files_commit ON git_commit_files(commit_sha);
CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_path ON source_manifest(repo_id, path);
CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_root ON source_manifest(repo_id, source_root);
CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_role ON source_manifest(repo_id, source_role);
CREATE INDEX IF NOT EXISTS idx_source_manifest_symbols_file ON source_manifest_symbols(file_id);
CREATE INDEX IF NOT EXISTS idx_source_manifest_tests_file ON source_manifest_tests(file_id);
CREATE INDEX IF NOT EXISTS idx_source_manifest_imports_file ON source_manifest_imports(file_id);

CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
  artifact_id UNINDEXED,
  title,
  body,
  source_path,
  tokenize='unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS artifact_sections_fts USING fts5(
  section_id UNINDEXED,
  artifact_id UNINDEXED,
  heading_path,
  title,
  body,
  tokenize='unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS source_manifest_fts USING fts5(
  file_id UNINDEXED,
  path,
  path_terms,
  source_root,
  language,
  source_role,
  symbols,
  test_names,
  imports,
  tokenize='unicode61'
);
