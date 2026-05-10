# Implementation Plan: Probabilistic related specs (CLI)

**Branch**: `001-discover-related-specs` | **Date**: 2026-05-10 | **Spec**: [spec.md](./spec.md)

**Implementation target**: [github.com/devspecs-com/devspecs-cli](https://github.com/devspecs-com/devspecs-cli) (`go 1.25`, Cobra, `modernc.org/sqlite`)

**Technical checklist**: [`testdata/samples/cursor/probabilistic_related_specs_481c4b3f.plan.md`](../../../cursor/probabilistic_related_specs_481c4b3f.plan.md) — schema v4 (`artifact_file_links`, `work_sessions`), fill `artifact_revisions.git_commit` on scan, store APIs (`UpsertFileLink`, `RelatedArtifactsForFile`, workon CRUD), `internal/mining`, `ds workon` / `ds mine` / `ds related`, generalized Git hooks (`post-commit`, `post-checkout`, `post-merge`, `post-rewrite`), regression + fixture coverage.

---

## Summary

Expose **probabilistic “likely related specs”** for repository files via new CLI commands grounded in **persisted evidence** and optional **Git hooks**. Persist evidence in SQLite **schema version 4** with two new constructs: **`artifact_file_links`** (many evidence rows per artifact + normalized file path, upserted idempotently) and **`work_sessions`** (associates repo/worktree/branch + HEAD with one active artifact during `ds workon`). **`ds mine`** collects git/text signals plus active work-session context and writes links; **`ds related <file>`** aggregates per-artifact scores (additive, cap 1.0) and maps to high/medium/low buckets with explainable evidence lines. **Scan** must populate **`artifact_revisions.git_commit`** for new revisions so history-oriented signals stay aligned with stored metadata. **Hooks** generalize `ds init --hooks` to install all four hook types with quiet + best-effort semantics from [PLAN.md](../../../codex/PLAN.md). See [research.md](./research.md) for locked decisions; [data-model.md](./data-model.md) for entities; [contracts/](./contracts/) for CLI/JSON contracts; [quickstart.md](./quickstart.md) for implementer workflow.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod` in repository root)  
**Primary Dependencies**: `github.com/spf13/cobra`, `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `github.com/oklog/ulid/v2`  
**Storage**: SQLite via `internal/store` (schema in `internal/store/schema.sql`, version gate in `internal/store/store.go`)  
**Testing**: `go test ./...`; package tests under `internal/store`, `internal/commands`, `internal/scan`, etc.; git-backed temp repos for command/fixture tests  
**Target Platform**: Developer workstations (Windows/macOS/Linux), Git CLI available in test scenarios  
**Project Type**: CLI binary `ds` (`cmd/ds/main.go`) with internal packages  
**Performance Goals**: `ds mine --recent` suitable for post-commit hooks (sub-second to low tens of seconds on typical repos); `ds mine --all` bounded by explicit caps (commits/files) to avoid multi-minute runs on huge histories  
**Constraints**: Hook output uses `--quiet`; JSON field names stable for scripts; schema bump follows existing rebuild-on-version-mismatch behavior  
**Scale/Scope**: v1 omits daemon, PR APIs, embeddings, LLM matching (per product PLAN)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` is still a **placeholder template** (unratified). Until a real constitution lands, compliance is asserted against this feature’s specification and codex PLAN:

| Gate | Status |
|------|--------|
| Test-backed behavior for store aggregation, hooks idempotency, and CLI JSON | Pass — required by spec + PLAN §Tests |
| No scope creep beyond probabilistic linking + hooks in v1 | Pass |
| Prefer thin commands; domain logic in `internal/mining`, persistence in `internal/store` | Pass |

**Post design**: Contracts and quickstart encode JSON stability expectations and schema ownership; no new constitution conflicts identified.

## Project Structure

### Documentation (this feature)

```text
specs/001-discover-related-specs/
├── plan.md           # This file
├── research.md       # Phase 0
├── data-model.md     # Phase 1
├── quickstart.md     # Phase 1
├── contracts/        # Phase 1
│   └── cli-ds-related.md
└── spec.md
```

### Source Code (repository root)

```text
cmd/ds/
└── main.go                    # Register workon, mine, related

internal/commands/
├── init.go                     # Multi-hook install (post-commit, post-checkout, post-merge, post-rewrite)
├── scan.go                     # Reference: --quiet pattern for mine
├── resume.go                   # Pattern for new cobra commands
├── ...                         # Existing commands
├── workon.go                   # NEW
├── mine.go                     # NEW
└── related.go                  # NEW

internal/store/
├── store.go                    # Bump SchemaVersion to 4
├── schema.sql                   # artifact_file_links, work_sessions, indexes
├── queries.go                   # UpsertFileLink, RelatedArtifactsForFile, workon helpers
└── store_test.go

internal/scan/
└── scan.go                     # Pass HeadCommit into insertRevision (+ tests)

internal/repo/
└── repo.go                     # Merge-base, diff helpers, symbolic-ref / default branch

internal/mining/
├── normalize.go               # Path normalization (align with adapters)
├── merge.go                    # additive + cap pure function (+ unit tests)
└── collectors*.go             # Git + text collectors (split as needed)

internal/adapters/              # Existing path/layout patterns reused by normalization
testdata/ ...
```

**Structure Decision**: Single Go module CLI; **new package `internal/mining`** holds collection + scoring helpers; **`internal/store`** remains source of truth for persistence; **`internal/commands`** orchestrate only.

## Technical workstream (aligned with checklist)

1. **Schema v4**: Bump version; add `artifact_file_links` and `work_sessions` with UNIQUE upsert key on `(artifact_id, file_path, evidence_type, evidence_value)`; indexes on `file_path`, `artifact_id`, and optional `(repo_id, branch)` columns only if queries require them without joins (per implementation plan tradeoff note); update `store_test` DDL expectations.
2. **Scan / `git_commit`**: Thread `repo.HeadCommit(repoRoot)` into `insertRevision` and any revision update path; regression test that commit is non-empty in a git-backed run.
3. **Store API**: `UpsertFileLink`; `RelatedArtifactsForFile(repoRoot, normalizedPath, includeLow bool)` grouping by artifact, additive sum capped at 1.0, evidence list for explainability, sort descending; workon: end open sessions for triple, start, get active, clear.
4. **`internal/mining`**: Normalize paths; implement confidence merge (pure); git signals via extended `repo` (merge-base with default branch, changed files in range, commit message ID regex, same-commit spec vs code pairing); text signals (`spec_mentions_file`, `todo_mentions_file`, `same_directory`); emit `workon_branch` when session active — **confidence constants must match codex PLAN.md §Mining Behavior exactly**.
5. **Commands**: Cobra wiring in `cmd/ds/main.go`; `--quiet` on `mine`; `related` default high+medium, `--all` includes low; `--json` on `mine` and `related` with fields documented in contracts.
6. **Hooks**: Refactor hook marker/install to multiple hook names; script bodies per codex PLAN; idempotent re-install; extend `freshness_test` / `init` tests for all hooks + double `init --hooks`.
7. **Tests**: Store (version, DDL, upsert idempotency, aggregation); commands in temp git repos; git fixtures for scenarios in PLAN; full `go test ./...` + golden JSON updates only when intentional.

## Risks (from implementation plans)

- **`--all` mining**: enforce max commits / max files; document behavior when truncated.
- **Worktree path**: v1 may set `worktree_root == repo_root`; document limitation if linked worktrees unsupported.
- **False positives**: CLI copy remains “likely related,” evidence strings stay honest.

## Complexity Tracking

> No constitution violations requiring justification. New `internal/mining` package is justified to keep commands thin and merge logic unit-testable in isolation.
