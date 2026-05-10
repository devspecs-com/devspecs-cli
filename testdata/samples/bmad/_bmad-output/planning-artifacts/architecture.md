---
stepsCompleted:
  - step-direct-authoring-complete
inputDocuments:
  - prd.md
workflowType: architecture
project_name: devspecs-cli
user_name: Brenn
date: '2026-05-10'
topic: 'Probabilistic related specs (schema v4)'
---

# Architecture — Probabilistic related specs (devspecs-cli)

This document specifies how to implement **related specs** (evidence + mining + work sessions) in the existing Go codebase using the **global SQLite store** pattern (`internal/store`, single DB path, WAL). Behavioral and schema requirements follow the product plans; **where plans differ on physical schema**, this doc follows the **concrete mapping onto `internal/store`, `internal/scan`, `internal/commands`, `internal/repo`, and hooks** (minimal columns on `artifact_file_links`, join `artifacts`/`repos` for repo scoping unless measurement proves otherwise).

**Out of scope:** Redesigning unrelated `ds` commands (`list`, `show`, `link` semantics, scan adapter behavior beyond revision/git metadata, etc.).

---

## 1. Context and dataflow

```mermaid
flowchart LR
  subgraph writers
    scan[ds scan]
    mine[ds mine]
    workon[ds workon]
  end
  subgraph store_db[(SQLite global DB)]
    rev[artifact_revisions]
    links[artifact_file_links]
    sess[work_sessions]
    art[artifacts]
    repos[repos]
  end
  subgraph readers
    related[ds related]
  end
  scan --> rev
  scan --> art
  mine --> links
  workon --> sess
  sess -.-> mine
  related --> links
  related --> art
  links --> art
  art --> repos
  hooks[git hooks] --> scan
  hooks --> mine
```

| Path | Role | Writes evidence | Writes sessions | Writes revisions |
|------|------|-----------------|-----------------|------------------|
| `ds scan` | Index artifacts | No | No | Yes (`artifact_revisions`, including **`git_commit`**) |
| `ds mine` | Collect signals | **Yes** (`artifact_file_links`) | No | No |
| `ds workon` | Declare focus artifact | No (indirect: enables `workon_branch` rows when **mine** runs) | **Yes** (`work_sessions`) | No |
| `ds related` | Rank specs for a file | **Read-only** | Read-only (optional: show active session in UX only) | No |

---

## 2. Migrations and store version

**Current behavior:** `internal/store/store.go` defines `SchemaVersion` and `(*DB).migrate()` applies embedded `schema.sql`, then records version in `schema_migrations`. If `maxVersion < SchemaVersion` **or** (on existing installs) the stored version is behind, the code enforces **`ds scan --rebuild`** / delete DB — **no incremental migrations** today.

**Decision:** Bump **`SchemaVersion` to 4** in `store.go`; extend **`internal/store/schema.sql`** with new tables and indexes; update **`store_test`** table/version expectations. Same **rebuild gate** as v3: old DBs must rebuild; do **not** introduce a separate migration framework in this slice.

**Failure mode:** Users on v3 see the existing error path asking for rebuild once they upgrade the binary. Document in release notes.

---

## 3. Schema additions (v4)

### 3.1 `artifact_file_links`

**Purpose:** Many rows per **(artifact, file_path)** describing *why* that file might relate to that DevSpec. This is the **only** table used for file↔artifact attribution in v1 (do **not** overload `links` for this).

**Columns (logical):**

| Column | Notes |
|--------|--------|
| `artifact_id` | FK → `artifacts(id)` |
| `file_path` | **Normalized** repo-relative path, `/` separators, stable string used in upsert key |
| `evidence_type` | One of the v1 type constants (below) |
| `evidence_value` | Short human-readable / debug string (commit id, slug, matched snippet id, etc.). If uniqueness is threatened by length, **hash or truncate** for the key only (implementation detail); keep display string derivable or store bounded text |
| `confidence` | Per-row weight from fixed table (below) |
| `first_observed_at` | Set on insert |
| `last_observed_at` | Bumped on upsert conflict |

**Uniqueness / upsert:**  
`UNIQUE(artifact_id, file_path, evidence_type, evidence_value)`  
On conflict: **update** `last_observed_at` (and `confidence` if the constant for that type ever changed — unlikely in v1).

**Repo scoping (global DB):**  
Do **not** rely on `file_path` alone across repos. **`ds related` and `ds mine` queries must scope** via `JOIN artifacts a ON a.id = artifact_file_links.artifact_id JOIN repos r ON a.repo_id = r.id WHERE r.root_path = ?` using the **same canonical `root_path`** convention as existing `queries.go` (see `GetRepoIDByRoot` / list filters).

**Indexes:**  
- `file_path` (lookup for `related`)  
- `artifact_id` (internal “files for artifact” helpers)  
Optional composite later if needed; **denormalize `repo_id` onto `artifact_file_links` only if** joins show up hot in profiles (Cursor plan: prefer minimal schema first).

### 3.2 `work_sessions`

**Purpose:** At most **one open** session per **`(repo_root, worktree_root, branch)`**; associates that context with an `artifact_id` and optional `head_commit` snapshot at start.

**Columns (logical):** `id`, `repo_root`, `worktree_root`, `branch`, `head_commit`, `artifact_id`, `started_at`, `ended_at` (NULL = active).

**Constraint:** Enforce “one open row per triple” with a **partial unique index** on `(repo_root, worktree_root, branch)` **WHERE `ended_at` IS NULL**, or equivalent application-level invariant plus store API that always **ends** prior open rows before insert.

**Path canonicalization:** Store **absolute** roots as strings consistent with `repos.root_path` and `repo.Detect` output (normalize with `filepath.Clean` / OS-specific absolute paths as today).

---

## 4. Evidence model

### 4.1 Evidence types (minimum set)

| Type | Role | Confidence |
|------|------|------------|
| `manual` | Explicit human/applied hint | 1.00 |
| `workon_branch` | Active `ds workon` + file in mined change set | 0.75 |
| `explicit_commit_ref` | DevSpec ID (full/short) appears in commit message | 0.50 |
| `same_commit` | Spec **source** file and code file changed in **same** commit | 0.45 |
| `branch_name_match` | Branch token/slug aligns with artifact title or source slug | 0.35 |
| `spec_mentions_file` | Artifact body contains path or basename | 0.30 |
| `commit_message_match` | Commit message token match (non-ID heuristic) | 0.20 |
| `same_directory` | Directory/module token affinity | 0.15 |
| `todo_mentions_file` | Todo text mentions path or basename | 0.10 |

**`manual` sourcing:** Mining heuristics do **not** need to emit `manual` in v1; reserve **`UpsertFileLink`** for a future explicit command or tooling. Aggregation logic **must** still include the type and weight so rows can appear when inserted.

### 4.2 Path normalization

**Single function** (new **`internal/mining`**, e.g. `NormalizeFilePath(repoRoot, path string) string`):

- Output **repo-relative** path.
- **Always `/`** separators (match adapter/source style used elsewhere).
- **Stable** under repeated mine runs: reject or strip `./`, collapse separators, case sensitivity follows **repository/OS** — document that case mismatch is a known false-negative on case-insensitive filesystems unless normalized via git paths.

**Upsert key:** Uses normalized `file_path` + `evidence_type` + `evidence_value` so repeated **`ds mine`** does not duplicate logical evidence.

---

## 5. Aggregation for `ds related`

**Rule:** For a given **`repo_root`** (for scoping) and **`file_path`** (normalized):

1. Load all `artifact_file_links` rows joined to artifacts/repos for that repo.
2. **Group by `artifact_id`**.
3. **`total = sum(confidence)`** per group, then **`total = min(total, 1.0)`** (additive cap).
4. Attach **ordered list of contributing evidence** (type, value, per-row confidence, timestamps) for CLI / JSON explainability.
5. Sort groups by **total** descending.

**Buckets** (on capped `total`):

| Bucket | Threshold |
|--------|-----------|
| high | ≥ 0.75 |
| medium | ≥ 0.45 |
| low | ≥ 0.20 |

**CLI default:** Show **high + medium** only; **`--all`** includes low.

**Implementation placement:** Core aggregation in **`internal/store`** as `RelatedArtifactsForFile(...)` (or equivalent name) returning structs ready for `--json`. Optionally extract **`CapAdditiveScore`** into **`internal/mining`** or **`internal/store`** as a **pure function** for table-driven tests (Cursor plan: test merge+cap in one place).

**Failure modes:**

- **No repo row** for cwd root → clear error (“run `ds scan` in this repo”).
- **File path outside repo** → error or refuse to normalize.
- **No links** → empty result, zero exit (document).

---

## 6. `ds workon` and work sessions

**Resolve artifact:** Reuse **`store.GetArtifact(idOrPrefix)`** (full id, short id, prefix — same ambiguity errors as `ds show`).

**Flow for `ds workon <id>`:**

1. `repo.Detect(cwd)` → `repo_root`, `CurrentBranch`; **`HeadCommit(repo_root)`**.
2. **`worktree_root`:** v1 **`worktree_root == repo_root`** (linked worktrees: document limitation; Cursor plan allows later `git rev-parse` refinement).
3. **`EndOpenSessions`** (or single-row update) for `(repo_root, worktree_root, branch)` then **`StartWorkon`** insert with `ended_at NULL`.

**`ds workon`:** Print active session artifact id/title/branch or “none”.

**`ds workon --clear`:** Set `ended_at` on active row for triple.

**Failure modes:** not a git repo, ambiguous artifact id, DB errors — mirror patterns from `internal/commands/show.go` / `tag.go`.

---

## 7. Scan / `artifact_revisions.git_commit`

**Today:** `insertRevision` in **`internal/scan/scan.go`** inserts only `id`, `artifact_id`, `content_hash`, `body`, `observed_at` — **`git_commit` column omitted** despite DDL.

**Change:** Thread **`repo.HeadCommit(repoRoot)`** (already in **`internal/repo/repo.go`**) into revision insert (and any **update** path if revisions are rewritten). When not in git or SHA empty, store **NULL** (acceptable).

**Tests:** Regression in scan or store: when run inside a temp git repo with commits, new revisions have **non-empty** `git_commit`.

---

## 8. Hooks

**Today:** **`internal/commands/init.go`** installs **only** `post-commit` with marker `# DevSpecs auto-index` and `ds scan --quiet --if-changed`.

**Change (per Cursor mapping):** Generalize to **multiple hook files** with **idempotent** marker behavior:

| Hook | Suggested script |
|------|------------------|
| `post-commit` | `ds scan --quiet --if-changed && ds mine --recent --quiet` |
| `post-checkout` | `ds scan --quiet` |
| `post-merge` | `ds scan --quiet && ds mine --recent --quiet` |
| `post-rewrite` | `ds scan --quiet && ds mine --recent --quiet` |

**Idempotency:** Either **one shared marker** checked per hook file or **per-hook marker suffix**; double `ds init --hooks` must **not** duplicate blocks.** Preserve **`|| true`** / stderr swallow pattern consistent with current trust model.

**Tests:** Extend **`internal/commands/freshness_test.go`** or dedicated **`init`** tests: all four hooks present, marker deduplication, mine invoked with `--quiet` in snippet.

---

## 9. Package and component map

| Concern | Package / file (target) |
|---------|-------------------------|
| DDL, `SchemaVersion`, open/migrate | `internal/store/store.go`, `internal/store/schema.sql` |
| `UpsertFileLink`, `RelatedArtifactsForFile`, work session CRUD | `internal/store/queries.go` (+ tests `queries_test.go`, `store_test.go`) |
| Path normalize, confidence constants, signal orchestration, pure merge/cap helpers | **`internal/mining`** (new): e.g. `normalize.go`, `signals.go`, `confidence.go`, `mine.go` |
| Git: merge-base, default branch, diff/file lists, log messages | **`internal/repo/repo.go`** (extend); mining calls repo helpers |
| Cobra: `related`, `mine`, `workon` | **`internal/commands/`** — new files mirroring `resume.go` patterns; **`--quiet`** on **mine** like `scan.go` |
| Registration | **`cmd/ds/main.go`** — `AddCommand` for three commands |
| Scan revision git SHA | **`internal/scan/scan.go`** — `insertRevision` signature + call sites |
| Hook install | **`internal/commands/init.go`** — refactor from single-file installer to multi-hook |

**`ds mine` (writer):** Builds candidate `(artifact_id, file_path, type, value, confidence)` tuples, normalizes paths, loads session state + artifact bodies/todos from **store** (read), **writes** via `UpsertFileLink`. Applies **`--recent`** (merge-base window, HEAD-ish) vs **`--all`** (capped commit/file limits — **document constants**, avoid unbounded history).

**`ds related` (reader):** Resolves file argument → normalize → `RelatedArtifactsForFile` → table/text + **`--json`** stable schema (field names locked in tests).

---

## 10. Failure modes and operational notes

| Scenario | Behavior |
|----------|----------|
| Binary upgraded; DB still v3 | Rebuild error until `ds scan --rebuild` (existing pattern) |
| Git binary missing / command fails | Mining skips or degrades git signals; prefer **non-fatal** mine with stderr in non-quiet mode |
| Huge repo + `mine --all` | **Hard cap** commits/files; exit 0 with partial coverage or document truncation in `--json` summary |
| Ambiguous artifact prefix on `workon` | Same as `GetArtifact` — fail with ambiguity message |
| Cross-repo `file_path` collision | Prevented by **repo_root scoping** in SQL |
| Worktree / path case | Document limitations; v1 equals worktree to repo root |

**Copy:** Output must remain **“likely related”**; evidence lines are **explanations**, not blame.

---

## 11. Testing strategy

1. **Store (fast):** v4 DDL present; upsert conflict updates `last_observed_at`; related query merges multiple rows and **caps** at 1.0; bucket boundaries; work session lifecycle (one open per triple).
2. **Pure helpers:** Confidence sums + cap; path normalization edge cases.
3. **Commands:** `workon` show/set/clear; `mine --recent --json` in **temp git repo** creates expected links; `related` text + JSON; `--all` exposes low bucket.
4. **Git fixtures (integration-style):** Scripted repos for: same-commit spec+code; branch slug match; active workon + touched files; spec/todo mentions file; commit message full/short id (`explicit_commit_ref`).
5. **Regression:** Existing **`go test ./...`**, golden JSON tests — **only** intentional field additions.

---

## 12. Open implementation choices (non-blocking)

- Exact **cap** numbers for `mine --all` (commits / files / message bytes).
- Whether to add **`repo_id`** column on `artifact_file_links` later for index-only plans.
- Future: **`manual`** evidence entry UX (subcommand vs editor integration).

---

## Handoff

Implement in order: **schema + store API** → **repo git helpers** → **`internal/mining`** → **commands** → **scan git_commit** → **hooks** → **fixtures + golden updates**. Align **`--json`** fields with tests early to avoid churn.
