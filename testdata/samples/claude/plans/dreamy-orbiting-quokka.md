# Probabilistic related specs — implementation plan

The 80/20 v0.1 slice for "likely related DevSpecs." This document specifies the feature and the concrete edits required to land it on the current codebase.

## What this slice actually ships

Three commands — `ds workon`, `ds mine`, `ds related` — backed by a new SQLite table (`artifact_file_links`) that stores ranked, evidence-typed associations between files and artifacts, plus a small `work_sessions` table that lets `ds workon <id>` declare "this branch is working on this artifact." `ds mine` is the only writer; `ds related <file>` is a read-only aggregator that sums evidence per artifact, caps at 1.0, and bucketises into high/medium/low. Git hooks expand from one (`post-commit`) to four (`post-commit`, `post-checkout`, `post-merge`, `post-rewrite`) so the local DB stays warm without a daemon.

What this slice deliberately is **not**: no PR provider integration, no embeddings, no LLM similarity, no background watcher. The promise to users is "likely related DevSpecs," not blame.

## How the pieces fit

```
ds scan        ─► artifact_revisions.git_commit (newly populated)
ds workon <id> ─► work_sessions (one open row per repo/worktree/branch)
ds mine        ─► artifact_file_links (many rows per artifact×file, one per evidence_type+value)
                  └── reads work_sessions to emit `workon_branch` evidence
ds related <f> ─► aggregates artifact_file_links → ranked artifacts + explanation
git hooks      ─► call ds scan + ds mine --recent on commit/checkout/merge/rewrite
```

Evidence rows are deliberately many-per-pair. The aggregation rule (additive sum, capped at 1.0) lives in **one** pure function so it's trivially unit-testable and the same rule applies whether the consumer is the CLI text formatter or the `--json` encoder.

## Task breakdown

A landable order — each task is independently testable, and the suggested grouping is the order I'd review PRs in.

- [ ] **schema-v4** — bump `SchemaVersion` to 4; add `artifact_file_links` and `work_sessions` to `schema.sql` with their indexes and uniqueness constraints; extend `store_test.go` table list and version assertion.
- [ ] **scan-git-commit** — thread `repo.HeadCommit` into `insertRevision` so `artifact_revisions.git_commit` is populated; add a regression test asserting non-empty value inside a temp git repo.
- [ ] **store-api** — implement `UpsertFileLink` (first `ON CONFLICT` upsert), `RelatedArtifactsForFile` (group + additive sum + cap + bucket), and the four work-session helpers (`StartWorkon` / `EndOpenSessions` / `GetActiveWorkon` / `ClearWorkon`); cover idempotency and aggregation.
- [ ] **repo-helpers** — add `MergeBase`, `DefaultBranch`, `ChangedFilesInRange`, `CommitMessage` to `internal/repo/repo.go`.
- [ ] **mining-package** — new `internal/mining` package: `Normalize`, per-evidence-type collectors with confidence constants, and a pure `Merge` / `Bucket` function with table-driven unit tests.
- [ ] **commands** — `ds workon [<id>] [--clear]`, `ds mine [--recent|--all] [--json] [--quiet]`, `ds related <file> [--all] [--json]`; register in `cmd/ds/main.go`. Reuse `store.GetArtifact` for ID resolution.
- [ ] **hooks** — generalize `init.go` hook installer to a `[]hookSpec` loop; install `post-commit`, `post-checkout`, `post-merge`, `post-rewrite` with the scripts in §Hooks; preserve marker-based idempotency and assert it in `init_test.go`.
- [ ] **integration-tests** — git-fixture scenarios (one per emitted evidence type) plus a regression sweep (`go test ./...`, golden JSON for `list` / `show` / `todos` / `resume` unchanged).

## Change set

### 1. Schema v4 — `internal/store/`

- **`store.go:19`** — bump `SchemaVersion` from `3` to `4`. The existing `migrate()` (lines 47–74) already errors with a `--rebuild` hint when the on-disk version is older, so no new migration code is needed; users running `ds scan --rebuild` get a fresh DB.

- **`schema.sql`** — append two tables and their indexes:

  ```sql
  CREATE TABLE artifact_file_links (
    id                 TEXT PRIMARY KEY,
    artifact_id        TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    file_path          TEXT NOT NULL,            -- repo-relative, forward slashes
    evidence_type      TEXT NOT NULL,
    evidence_value     TEXT NOT NULL DEFAULT '', -- e.g. commit SHA, branch name, todo text
    confidence         REAL NOT NULL,
    first_observed_at  TEXT NOT NULL,
    last_observed_at   TEXT NOT NULL,
    UNIQUE(artifact_id, file_path, evidence_type, evidence_value)
  );
  CREATE INDEX idx_afl_file ON artifact_file_links(file_path);
  CREATE INDEX idx_afl_artifact ON artifact_file_links(artifact_id);

  CREATE TABLE work_sessions (
    id              TEXT PRIMARY KEY,
    repo_root       TEXT NOT NULL,
    worktree_root   TEXT NOT NULL,
    branch          TEXT NOT NULL,
    head_commit     TEXT NOT NULL,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    started_at      TEXT NOT NULL,
    ended_at        TEXT
  );
  CREATE UNIQUE INDEX idx_ws_active
    ON work_sessions(repo_root, worktree_root, branch)
    WHERE ended_at IS NULL;
  ```

  The `UNIQUE(artifact_id, file_path, evidence_type, evidence_value)` constraint makes `INSERT ... ON CONFLICT DO UPDATE SET last_observed_at = excluded.last_observed_at` repeatable without dupes. The partial unique index on active work sessions guarantees at most one open session per branch.

- **`queries.go`** — add:
  - `UpsertFileLink(artifactID, filePath, evidenceType, evidenceValue string, confidence float64, now string) error` — uses `ON CONFLICT(artifact_id, file_path, evidence_type, evidence_value) DO UPDATE SET last_observed_at = excluded.last_observed_at, confidence = excluded.confidence`. This is the codebase's first proper conflict-target upsert; `InsertTag` at `queries.go:588` only uses `INSERT OR IGNORE`, which won't bump observed times.
  - `RelatedArtifactsForFile(filePath string, includeLow bool) ([]RelatedRow, error)` — `SELECT artifact_id, evidence_type, evidence_value, confidence FROM artifact_file_links WHERE file_path = ?`, then in Go: group by `artifact_id`, sum confidences (cap 1.0), bucketise, optionally filter low, sort score descending. Returns one row per artifact with an attached `[]Evidence` slice for explainability.
  - `StartWorkon(repoRoot, worktreeRoot, branch, headCommit, artifactID, now string) (sessionID string, err error)` — wraps `EndOpenSessions` then `INSERT`. One transaction.
  - `EndOpenSessions(repoRoot, worktreeRoot, branch, now string) error`.
  - `GetActiveWorkon(repoRoot, worktreeRoot, branch string) (*WorkSessionRow, error)` — returns nil when no session.
  - `ClearWorkon(repoRoot, worktreeRoot, branch, now string) error` — same as `EndOpenSessions` for that triple.

  Reuse `GetArtifact(idOrPrefix)` at `queries.go:149-199` for the full→short→prefix resolution `ds workon <id>` needs — no new resolution logic.

- **`store_test.go:18`** — append `"artifact_file_links"` and `"work_sessions"` to the table-presence list. The existing `TestMigrate_Idempotent` already asserts `MAX(version) == SchemaVersion`, so bumping the constant flows through. Add one test for upsert idempotency (insert same evidence twice → one row, `last_observed_at` advances) and one for `RelatedArtifactsForFile` aggregation (three evidence rows for one artifact → single result with summed score).

### 2. Populate `artifact_revisions.git_commit` — `internal/scan/scan.go`

`schema.sql:42` already declares `artifact_revisions.git_commit TEXT`, and `scan.go:76` already calls `repo.HeadCommit(repoRoot)` — the value just isn't threaded into `insertRevision` (`scan.go:182-188`, currently 5 columns). Extend the parameter list and the `INSERT` to include `git_commit`. Add a small scan test that runs inside a temp git repo and asserts the column is non-empty for newly inserted revisions.

### 3. Repo helpers — `internal/repo/repo.go`

Today the package exports `Detect`, `HeadCommit`, `ChangedFiles`. The mining engine needs four more, all thin `os/exec` wrappers in the same style:

- `MergeBase(repoRoot, ref string) (string, error)` — `git merge-base HEAD <ref>`.
- `DefaultBranch(repoRoot string) (string, error)` — try `git symbolic-ref refs/remotes/origin/HEAD`, fall back to checking `main` then `master`. Returns `""` cleanly when neither exists (CI / fresh repos).
- `ChangedFilesInRange(repoRoot, fromRef, toRef string) ([]string, error)` — `git diff --name-only fromRef..toRef`.
- `CommitMessage(repoRoot, ref string) (string, error)` — `git log -1 --format=%B <ref>`. One function for subject+body; the mining layer can split on `\n\n` if it ever needs to.

### 4. Mining engine — `internal/mining/` (new)

Keeps `internal/commands` thin. Three files:

- `mining/path.go` — `Normalize(repoRoot, path string) string` lifts the `filepath.Rel` + `filepath.ToSlash` pattern already used inline at `internal/adapters/markdown/markdown.go:47`. Centralising it here means future call sites get one rule.

- `mining/collectors.go` — one function per evidence type, each returning `[]Link{ArtifactID, FilePath, EvidenceType, EvidenceValue, Confidence}`:

  | Evidence type           | Confidence | Source                                                   |
  |-------------------------|-----------:|----------------------------------------------------------|
  | `manual`                | `1.00`     | (reserved for a future `ds link --file`; not emitted by mine in v1) |
  | `workon_branch`         | `0.75`     | active `work_sessions` row × current-branch changed files |
  | `explicit_commit_ref`   | `0.50`     | full or short artifact ID found in commit message       |
  | `same_commit`           | `0.45`     | spec source path **and** non-spec path in same commit   |
  | `branch_name_match`     | `0.35`     | branch name token-overlaps artifact title/source slug   |
  | `spec_mentions_file`    | `0.30`     | artifact body contains exact path or basename           |
  | `commit_message_match`  | `0.20`     | commit message tokens overlap artifact title slug       |
  | `same_directory`        | `0.15`     | file shares directory with a known artifact source      |
  | `todo_mentions_file`    | `0.10`     | artifact todo text contains path or basename            |

  Restate the constants as Go consts so a future tweak is one diff. Confidences are additive across rows for the same `(artifact, file)` pair, capped at 1.0 by the merge step (see "Aggregation rule" below).

- `mining/merge.go` — one pure function:

  ```go
  func Merge(rows []Link) []Result // group by ArtifactID, sum Confidence, cap at 1.0, attach evidence
  func Bucket(score float64) string // "high" >=0.75, "medium" >=0.45, "low" >=0.20, else ""
  ```

  Pure → easy table-driven unit tests for the cap and the bucket boundaries.

### 5. Commands — `internal/commands/`

Pattern-match the structure of `resume.go` (cobra command + flag bindings + a `runX` function that does the work). Three new files:

- `workon.go`
  - `ds workon <id>` — `repo.Detect` → `store.GetArtifact(id)` → `store.StartWorkon(...)` → `"Current branch <branch> is now associated with <id>."`
  - `ds workon` — print active session for current repo/worktree/branch, or "no active work session."
  - `ds workon --clear` — `store.ClearWorkon(...)`. Idempotent (no error if nothing was active).

- `mine.go`
  - Flags `--recent`, `--all`, `--json`, `--quiet` (mirroring `scan.go:25-49`).
  - `--recent`: changed files since merge-base with default branch, or HEAD's parent if no default branch detected.
  - `--all`: walk reachable history, capped (`maxCommits = 500`, `maxFilesPerCommit = 200`) so it never runs for minutes on a large repo. Document the cap in `--help`.
  - For each commit in scope: run all collectors, dedupe within the call, batch-upsert via `UpsertFileLink`.
  - `--json`: emit `{"new_links": N, "updated_links": N, "buckets": {"high": N, "medium": N, "low": N}}`.

- `related.go`
  - `ds related <file>` — normalize path, call `RelatedArtifactsForFile(path, includeLow=false)`, print one section per artifact with `<short_id> <title>  [bucket score]` followed by indented evidence lines. Never print a bare score without evidence — that's the explainability promise.
  - `--all`: pass `includeLow=true`.
  - `--json`: array of `{artifact_id, short_id, title, score, bucket, evidence: [{type, value, confidence}]}`. lowercase_snake_case to match `show.go:64-67` and `todos.go`.

- **Register in `cmd/ds/main.go:30-45`** — three more `rootCmd.AddCommand(...)` lines next to the existing fifteen.

### 6. Hooks — `internal/commands/init.go`

Today only `post-commit` is installed (`init.go:122`), guarded by `hookMarker = "# DevSpecs auto-index"` (`:92`) with the append-or-skip pattern at `:127-136`. Generalize:

```go
type hookSpec struct{ name, script string }

var hooks = []hookSpec{
    {"post-commit",   "ds scan --quiet --if-changed && ds mine --recent --quiet"},
    {"post-checkout", "ds scan --quiet"},
    {"post-merge",    "ds scan --quiet && ds mine --recent --quiet"},
    {"post-rewrite",  "ds scan --quiet && ds mine --recent --quiet"},
}
```

Loop over `hooks`, reuse the existing marker check per file, append `|| true` to every command line so a missing binary or scan failure never aborts a git operation. The idempotency contract stays: `ds init --hooks` run twice ends up with each hook installed exactly once.

## Aggregation rule (single source of truth)

For each `(file_path)` query, group `artifact_file_links` rows by `artifact_id`, sum `confidence` additively, cap at `1.0`, attach the underlying evidence rows. Bucket the capped score: `>=0.75` high, `>=0.45` medium, `>=0.20` low. `ds related` defaults to high+medium; `--all` includes low. Anything below `0.20` is dropped — the noise floor.

## Tests

- **Store** (`internal/store/store_test.go`): table presence for both new tables; `SchemaVersion == 4`; `UpsertFileLink` idempotency (same key twice → one row, advancing `last_observed_at`); `RelatedArtifactsForFile` aggregation (3 evidence rows for 1 artifact → one ranked result, score capped).
- **Mining** (`internal/mining/merge_test.go`): table-driven tests for `Merge` and `Bucket` — empty input, single row, multiple rows summing past 1.0 (verify cap), exact bucket boundaries (`0.7499`, `0.75`, `0.4499`, `0.45`, `0.1999`, `0.20`).
- **Commands** (`internal/commands/`): `workon` show / clear; `mine --recent --json` against a temp git repo; `related <file>` text and JSON. Reuse `setupGoldenEnv()` at `internal/commands/golden_test.go:81-113` for the temp-git harness; add new golden files masked via the existing `maskDynamic` helper.
- **Git fixtures**, one scripted scenario per evidence type that the mining engine emits:
  - same-commit spec+code → `same_commit`
  - branch named like artifact slug → `branch_name_match`
  - active `workon` session covering changed branch files → `workon_branch`
  - spec body mentions a file path → `spec_mentions_file`
  - commit message contains a short or full artifact ID → `explicit_commit_ref`
- **Hooks** (`internal/commands/init_test.go`): all four hooks present after `ds init --hooks`; running it twice doesn't duplicate marker lines.
- **Regression**: `go test ./...` clean; existing golden JSON for `list`, `show`, `todos`, `resume` unchanged.

## Risks and non-goals

- **History walk cost.** `--all` must respect the documented caps; without them, a large monorepo turns `ds mine --all` into a multi-minute stall. The caps are conservative defaults, not policy — easy to lift later.
- **Worktree assumption.** v1 treats `worktree_root == repo_root`. Linked worktrees still work (one row per branch), they just won't disambiguate on worktree path. Note this in `--help` for `ds workon`.
- **False positives are expected.** `same_directory` (`0.15`) will surface unrelated specs in busy directories; that's why it's low-bucket and hidden by default.
- **Migration is rebuild-only.** Existing v3 DBs require `ds scan --rebuild`. Same policy as the v2→v3 bump.
- **Out of scope:** PR provider hooks, embeddings, LLM similarity, watcher daemon, `ds blame` alias, public `ds files` command.

## Verification

```
go build ./...
go test ./...
```

Then in a scratch repo:

```
ds init --hooks                      # installs four hooks
ds init --hooks                      # idempotent — no duplicate marker lines
ds scan
ds workon <some-artifact-id>         # records branch↔artifact intent
git commit -am "touch a file"        # post-commit hook runs scan + mine
ds related path/to/file              # human-readable, with evidence lines
ds related path/to/file --json       # stable JSON, lowercase_snake_case keys
ds mine --recent --json              # bucket counts
ds workon --clear                    # ends the session
```

If all four hooks fire without aborting commits, `ds related` returns explainable evidence (not bare scores), and the v3 → v4 path errors cleanly with a `--rebuild` hint, the slice is shippable.
