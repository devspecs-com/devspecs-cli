# 80/20 Probabilistic Related Specs

## Summary

Build the first version around `ds related <file>`, `ds mine`, and `ds workon <id>`. The feature should promise “likely related DevSpecs,” not exact causal blame.

This slice will use explicit/manual hints, current-branch association, same-commit structural evidence, branch/title token affinity, and simple spec/todo text matching. No daemon, PR API, embeddings, or LLM matching in v1.

## Key Changes

- Add SQLite schema version 4 with:
  - `artifact_file_links`: ranked file-to-artifact evidence rows with `artifact_id`, normalized `file_path`, `evidence_type`, `evidence_value`, `confidence`, `first_observed_at`, `last_observed_at`.
  - `work_sessions`: repo/worktree/branch-to-artifact association for `ds workon`.
  - indexes on `file_path`, `artifact_id`, `repo_id + branch`, and a uniqueness constraint for repeatable evidence upserts.
- Extend scan revision metadata so new artifact revisions record the current `git_commit`; currently `artifact_revisions.git_commit` exists but scan inserts leave it empty.
- Add store query helpers for:
  - upserting file-link evidence without duplicating rows,
  - resolving related artifacts for a file,
  - setting/getting/clearing active `workon` sessions,
  - listing likely files for an artifact internally, even if no public `ds files` command ships yet.

## Commands

- `ds workon <id>`
  - Resolves full ID, short ID, or prefix.
  - Associates the current repo root, worktree root, current branch, current HEAD, and artifact ID.
  - Ends any previous open session for the same repo/worktree/branch.
  - Output: “Current branch <branch> is now associated with <id>.”
- `ds workon`
  - Shows the current active artifact for this repo/worktree/branch, if any.
- `ds workon --clear`
  - Ends the active session for the current repo/worktree/branch.
- `ds mine`
  - Default: mine current repo using recent/local signals.
  - `--recent`: inspect only current `HEAD` and nearby commits, suitable for hooks.
  - `--all`: inspect broader reachable history, capped conservatively.
  - `--json`: return summary counts and evidence bucket counts.
- `ds related <file>`
  - Shows high and medium confidence results by default.
  - `--all`: include low confidence results.
  - `--json`: output machine-readable results with artifact, confidence, bucket, and evidence list.

## Mining Behavior

- Normalize file paths to slash-separated repo-relative paths before storage and lookup.
- Evidence types for this slice:
  - `manual`
  - `workon_branch`
  - `explicit_commit_ref`
  - `same_commit`
  - `branch_name_match`
  - `commit_message_match`
  - `spec_mentions_file`
  - `todo_mentions_file`
  - `same_directory`
- Confidence scoring:
  - `manual`: `1.00`
  - `workon_branch`: `0.75`
  - explicit DevSpec ID in commit message: `0.50`
  - spec source and code file changed in same commit: `0.45`
  - branch name matches artifact title/source slug: `0.35`
  - spec body mentions exact file path/name: `0.30`
  - commit message token match: `0.20`
  - same directory/module token match: `0.15`
  - todo mentions file path/name token: `0.10`
  - combine additively per artifact/file and cap at `1.0`.
- Confidence buckets:
  - high: `>= 0.75`
  - medium: `>= 0.45`
  - low: `>= 0.20`
- `ds mine --recent` should:
  - inspect current branch changed files since merge-base with default branch when available, otherwise recent commits,
  - apply `workon_branch` evidence when an active work session exists,
  - inspect commits touching spec source files and code files together,
  - scan commit messages for full artifact IDs and short IDs,
  - scan current artifact bodies/todos for exact path/name mentions.
- `ds related <file>` should aggregate evidence rows by artifact and print explainable evidence lines, never just a bare score.

## Hook Integration

- Update `ds init --hooks` to install or append:
  - `post-commit`: `ds scan --quiet --if-changed && ds mine --recent --quiet`
  - `post-checkout`: `ds scan --quiet`
  - `post-merge`: `ds scan --quiet && ds mine --recent --quiet`
  - `post-rewrite`: `ds scan --quiet && ds mine --recent --quiet`
- Keep hooks best-effort with `|| true`, matching the current trust model.

## Tests

- Store tests:
  - schema version bumps to 4,
  - new tables/indexes exist,
  - file-link upsert updates `last_observed_at` and does not duplicate evidence,
  - related query aggregates multiple evidence rows into one ranked artifact result.
- Command tests:
  - `ds workon <id>`, `ds workon`, and `ds workon --clear`.
  - `ds mine --recent --json` creates expected links from a temporary git repo.
  - `ds related <file>` shows likely related artifacts and evidence text.
  - `ds related <file> --json` is valid stable JSON.
- Git fixture scenarios:
  - commit touches spec file and code file in same commit.
  - branch name resembles OpenSpec change slug.
  - active `ds workon` session links changed branch files.
  - spec body/todo mentions a file path/name.
- Regression tests:
  - existing `ds scan`, `ds list`, `ds show`, `ds link`, and golden JSON tests still pass.
  - hooks remain idempotent when `ds init --hooks` runs more than once.

## Assumptions

- Public lookup command is `ds related <file>`; no `ds blame` alias in the first slice.
- `ds workon + ds mine` is included in the first 80/20 implementation.
- Existing generic `ds link` remains for external links; file attribution uses the new `artifact_file_links` table instead of overloading `links`.
- Old DBs may continue using the project’s current rebuild-required migration behavior unless a separate migration framework is introduced later.
- No watcher, daemon, PR-provider integration, embeddings, or LLM similarity in this slice.
