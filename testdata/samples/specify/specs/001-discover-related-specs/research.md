# Research: Probabilistic related specs

**Feature**: [spec.md](./spec.md)  
**Date**: 2026-05-10

Consolidated decisions for implementation on `devspecs-cli`. Source materials: [spec](./spec.md), [codex PLAN.md](../../../codex/PLAN.md), [cursor implementation checklist](../../../cursor/probabilistic_related_specs_481c4b3f.plan.md).

---

## R-1: Evidence types and confidence weights

**Decision**: Use the **exact** evidence type strings and per-type confidence values from [codex PLAN.md §Mining Behavior](../../../codex/PLAN.md).

**Rationale**: Spec FR-003/FR-004 define buckets and merge rule; codex PLAN locks the per-signal weights so mining output is stable and testable.

**Alternatives considered**: Tunable config file — rejected for v1 (adds surface area before metrics exist).

| evidence_type | confidence |
|---------------|------------|
| `manual` | 1.00 |
| `workon_branch` | 0.75 |
| `explicit_commit_ref` | 0.50 |
| `same_commit` | 0.45 |
| `branch_name_match` | 0.35 |
| `spec_mentions_file` | 0.30 |
| `commit_message_match` | 0.20 |
| `same_directory` | 0.15 |
| `todo_mentions_file` | 0.10 |

**Aggregation**: For a given `(artifact_id, file_path)`, sum applicable row confidences, **cap at 1.0**, then apply buckets: high ≥ 0.75, medium ≥ 0.45, low ≥ 0.20; below 0.20 treated as non-match for default output.

---

## R-2: Path normalization

**Decision**: Normalize to **repo-relative** paths with **forward slashes** before upsert and lookup; align with patterns already used for source paths in `internal/adapters`.

**Rationale**: Single join key across mining, store, and `ds related` regardless of OS path separators.

**Alternatives considered**: Store absolute paths — rejected (breaks portability and duplicates per machine).

---

## R-3: Default branch for merge-base

**Decision**: Prefer `git symbolic-ref refs/remotes/origin/HEAD` when available; fallback to **`main` / `master` heuristic** consistent with existing repo helpers.

**Rationale**: `ds mine --recent` needs a stable base for “changed since merge-base” per codex PLAN.

---

## R-4: SQLite schema and migration

**Decision**: Bump **`SchemaVersion` to 4** in `internal/store/store.go`; extend `schema.sql` with `artifact_file_links` and `work_sessions` per checklist; rely on **existing rebuild-on-version-mismatch** behavior (same pattern as prior schema bumps).

**Rationale**: Matches current project practice; no separate migration framework in scope.

**Alternatives considered**: Incremental ALTER migrations — not required unless product policy changes.

---

## R-5: Hook script bodies and failure policy

**Decision**: Install four hooks with these command lines (quiet modes):

| Hook | Script |
|------|--------|
| `post-commit` | `ds scan --quiet --if-changed && ds mine --recent --quiet` |
| `post-checkout` | `ds scan --quiet` |
| `post-merge` | `ds scan --quiet && ds mine --recent --quiet` |
| `post-rewrite` | `ds scan --quiet && ds mine --recent --quiet` |

Append **`|| true`** (or equivalent best-effort) so hooks never block developer workflows — matches codex PLAN “best-effort” trust model and existing conventions.

---

## R-6: Packaging mining logic

**Decision**: Introduce **`internal/mining`** for collectors + pure merge scoring; **`internal/commands`** delegate to mining + store.

**Rationale**: Satisfies testability (unit tests on merge), keeps Cobra adapters thin.

**Alternatives considered**: All logic inline in commands — rejected (harder to test and reuse from future automation).

---

## R-7: JSON output stability

**Decision**: Define field names explicitly in tests and document in [contracts/cli-ds-related.md](./contracts/cli-ds-related.md); additive fields only across minor revisions unless semver/announcement policy says otherwise.

**Rationale**: FR-007 and spec assumptions require script-safe structured output.

---

## Resolved unknowns

No **`NEEDS CLARIFICATION`** items remain at planning time; open product tradeoffs (**optional** `repo_id`/`branch` columns on links, linked worktrees) are explicitly deferred or documented as v1 limitations in [plan.md](./plan.md).
