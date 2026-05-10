---

description: "Task list template for feature implementation"
---

# Tasks: Probabilistic related specs (CLI)

**Input**: Design documents from `/specs/001-discover-related-specs/`  
**Implementation root**: [github.com/devspecs-com/devspecs-cli](https://github.com/devspecs-com/devspecs-cli) repository — **all paths below are relative to that module root**, not this `testdata/samples/specify` preview tree.

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-ds-related.md, quickstart.md

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no unresolved dependencies).
- **[Story]**: Applies to user-story phases [US1]–[US4] only.
- Paths must identify real files/packages in `devspecs-cli`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Workspace readiness before schema or CLI edits.

- [ ] T001 Run baseline **passing** `go test ./...` at devspecs-cli repository root (`github.com/devspecs-com/devspecs-cli`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: SQLite schema v4 + store/read APIs + scan metadata + deterministic merge utilities. **⚠️ No user-story implementation should start until Phase 2 is complete.**

- [ ] T002 Bump `SchemaVersion` to **4** in `internal/store/store.go` (reuse existing rebuild-on-mismatch semantics).
- [ ] T003 Add `artifact_file_links` + `work_sessions` tables with indexes, FKs consistent with repo conventions, and `UNIQUE(artifact_id,file_path,evidence_type,evidence_value)` upsert key in `internal/store/schema.sql` per `testdata/samples/specify/specs/001-discover-related-specs/data-model.md`
- [ ] T004 Update `internal/store/store_test.go` (and any DDL snapshot helpers) so schema/version expectations match v4 tables.
- [ ] T005 Implement `UpsertFileLink(...)` with `INSERT ... ON CONFLICT DO UPDATE` (refresh `last_observed_at`; confidence updates if policy dictates) in `internal/store/queries.go`
- [ ] T006 Implement `RelatedArtifactsForFile(repoRoot,normalizedPath,includeLow bool)` (group-by-artifact additive sum capped at **1.0**, attach evidence slice, descending sort by score, filter buckets) in `internal/store/queries.go`
- [ ] T007 Implement work-session primitives `EndOpenSessions`/`StartWorkon`/`GetActiveWorkon`/`ClearWorkon` in `internal/store/queries.go` enforcing one active `(repo_root,worktree_root,branch)` semantics.
- [ ] T008 Add store/regression coverage for upsert idempotency and multi-evidence aggregation/ranking in `internal/store/store_test.go` or paired `queries_test.go`
- [ ] T009 [P] Pass `repo.HeadCommit(repoRoot)` into `insertRevision` (and revision update mirrors) inside `internal/scan/scan.go`
- [ ] T010 [P] Add regression test ensuring persisted revisions store non-empty `git_commit` when scanning within a Git repo under `internal/scan/` test files
- [ ] T011 Implement additive-merge + clamp + tier mapping helpers in `internal/mining/merge.go` exactly matching **`testdata/samples/codex/PLAN.md`** §Mining Behavior thresholds.
- [ ] T012 [P] Add unit tests validating merge clamps + bucket breakpoints in `internal/mining/merge_test.go`
- [ ] T013 Implement repo-relative normalization utilities in `internal/mining/normalize.go` aligning with `internal/adapters/` path normalization patterns

**Checkpoint**: Foundation ready → user-story work can spawn in parallel (if staffed) while respecting dependencies below.

---

## Phase 3: User Story 1 – Related command (Priority: P1) 🎯 MVP

**Goal**: `ds related <path>` exposes ranked artifact matches with textual evidence rows and deterministic `--json` shaped per `testdata/samples/specify/specs/001-discover-related-specs/contracts/cli-ds-related.md`; default hides low bucket (`--all` shows low).

**Independent Test**: Seed links via store helpers/fixtures inside a temp repo, invoke `related`, assert human buckets + `--json` payload without requiring mining.

- [ ] T014 [US1] Implement `related` UX (path discovery, normalization, aggregated scores, textual evidence formatting, `--all` gate) orchestrating store queries inside `internal/commands/related.go`
- [ ] T015 [US1] Extend `internal/commands/related.go` `--json` output to match negotiated field names/schema notes in `contracts/cli-ds-related.md`
- [ ] T016 [US1] Register `related` Cobra command in `cmd/ds/main.go`
- [ ] T017 [P] [US1] Add CLI regression tests covering text + `--json` results under `internal/commands/*_test.go` using seeded `artifact_file_links` rows

---

## Phase 4: User Story 2 – Mine command (Priority: P1)

**Goal**: `ds mine` walks git/text/workon-derived signals (`--recent` vs capped `--all`) and persists evidence rows through `UpsertFileLink`; supports `--quiet` + structured `--json` summaries.

**Independent Test**: Script temp Git history + artisan bodies; run mine, assert persisted links plus JSON summary buckets.

- [ ] T018 [P] [US2] Implement merge-base / diff listing / symbolic-ref helpers for default-branch discovery inside `internal/repo/repo.go`
- [ ] T019 [US2] Implement git-centric collectors (`same_commit`, `explicit_commit_ref`, `branch_name_match`, `commit_message_match`, capped ranges) emitting normalized paths in `internal/mining/` (`collectors_git.go` split optional)
- [ ] T020 [P] [US2] Implement text collectors (`spec_mentions_file`, `todo_mentions_file`, `same_directory`) emitting evidence rows inside `internal/mining/` (`collectors_text.go` split optional)
- [ ] T021 [US2] Wire collectors + transactional upserts behind `ds mine` flags (`--recent`, `--all`, caps, `--quiet`, `--json`) in `internal/commands/mine.go`
- [ ] T022 [US2] Register `mine` command in `cmd/ds/main.go`
- [ ] T023 [P] [US2] Add temp-repo regression tests asserting `mine --recent --json` writes expected evidence in `internal/commands/*_test.go` or partnered `internal/mining/*_test.go`

---

## Phase 5: User Story 3 – Workon command & session-linked mining (Priority: P2)

**Goal**: `ds workon <id>` / `--clear` / bare status associate branch+HEAD artifact focus; miner emits `workon_branch` evidence when active sessions overlap mined files/commits.

**Independent Test**: Start session, mutate branch files via fixtures, rerun `mine --recent`, confirm `workon_branch` evidences persisted; `--clear` ends session deterministically.

- [ ] T024 [US3] Implement work session lifecycle UX (resolve artifact IDs, Detect repo/worktree HEAD, informational strings) inside `internal/commands/workon.go`
- [ ] T025 [US3] Register `workon` command in `cmd/ds/main.go`
- [ ] T026 [P] [US3] Hook active session lookups into miner path so qualifying touched files enqueue `workon_branch` evidences referencing `testdata/samples/codex/PLAN.md` weights inside `internal/mining/` / `internal/commands/mine.go`
- [ ] T027 [US3] Extend automated tests validating workon transitions + interplay with miner under `internal/commands/*_test.go`

---

## Phase 6: User Story 4 – Git hook automation (Priority: P3)

**Goal**: `ds init --hooks` installs `post-commit`, `post-checkout`, `post-merge`, `post-rewrite` scripts invoking quiet `scan`/`mine`, remains marker-idempotent (`|| true` semantics per codex PLAN).

**Independent Test**: Run installer twice → single hook snippets; asserts contain expected chained commands referencing `contracts/cli-ds-related.md`/PLAN wording.

- [ ] T028 [US4] Refactor generalized hook marker append logic for four hook targets + script payloads inside `internal/commands/init.go`
- [ ] T029 [P] [US4] Expand reliability tests asserting hook bodies + duplication resistance in `internal/commands/freshness_test.go` (or split `internal/commands/init_test.go` when cleaner)

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T030 [P] Add git fixture choreography covering PLAN scenarios (`same-commit` spec/code, slug branch, explicit message IDs, textual mentions, workon-linked miner runs) beside existing `testdata/` harness conventions in `devtools`/`internal/commands`.
- [ ] T031 [P] Refresh documentation in repo-root `README.md` outlining new commands/hook cadence referencing user-facing summaries (optional but recommended post-implementation polish).
- [ ] T032 Run full `go test ./...`; only modify golden snapshots / serialized JSON artifacts when adding additive fields deliberately (inspect existing golden paths under repository before editing).

---

## Dependencies & Execution Order

### Phase dependencies

| Phase | Depends on |
|-------|-------------|
| Setup (Phase 1) | None |
| Foundational (Phase 2) | Phase 1 |
| US1/US2/US3/US4 | Phase 2 complete |
| Polish (Phase 7) | Desired user stories landed |

### User story sequencing

```text
Foundation (T002-T013 covering store/scan/mining prerequisites)
├── US2 Mine (writes evidence)
└── parallelizable read path MVP (US1) once store query complete (fixtures seed data if mine not ready)
Once US2 exists fully, prioritize expanding tests using living miner feeds.
├── US3 Workon layers atop miner + commands (after US2 command wiring minimally shares session lookups)
└── US4 Hooks last (assume `scan`, `mine` CLI stable incl. `--quiet`)
```

### Story dependency nuance

- **US1** can ship after foundation by **seeding SQLite rows** manually in tests—even before US2—to honor spec’s “Independent Test”.
- Still, **production value** emerges once **US2** writes real evidence continually.
- **US3** requires **store session APIs** (`T007`) plus **miner** (`T019-T026`).
- **US4** consumes stable `mine`/`scan` flags finalized in preceding phases.

### Within-story order

Miner helpers (`T019-T020`) precede orchestration (`T021`). Hooks depend on finalized CLI UX.

---

## Parallel execution examples

### Foundation

```bash
# After T008 completes, shard remaining packages:
Tasks: "T009 Thread HeadCommit … internal/scan/scan.go"
Tasks: "T010 regression tests … internal/scan/"
Tasks: "T012 mining merge tests … internal/mining/merge_test.go" # after merge helper exists T011 preceding
```

### User Story 1

```bash
Tests (T017) can start parallel to docs once command skeleton compiles alongside seeded fixtures.
```

### User Story 2

```bash
Tasks: "T018 repo helpers … internal/repo/repo.go"
Tasks: "T020 textual collectors … internal/mining/"
# After interfaces settled, finalize orchestration (`T021`).
```

### User Story 4

```bash
Tasks: "T029 enlarge hook tests … freshness_test.go" only after refactor (`T028`) stabilizes filenames/markers.
```

---

## Parallel opportunities summary

| Area | Parallel-ready tasks |
|------|---------------------|
| Foundation | Scan (`T009`/`T10`) versus merge tests (`T12`) once merge helper landed (`T11`) |
| US1 | Test harness (`T17`) isolated files |
| US2 | Repo helpers (`T18`) vs text miner (`T20`) before orchestration merges |
| US3 | Session-aware miner augmentation (`T26`) after lifecycle command exists (`T24-25`) |
| US4 | Test expansion (`T29`) parallelizable after refactor (`T28`) |
| Polish | Fixtures/doc/test sweeps (`T30`/`T31`/`T32`) |

---

## Implementation strategy

### MVP first

1. Complete Setup + Foundational (T001–T013).  
2. Deliver **US1** with seeded evidence (T014–T017) → validates UX + JSON contract.  
3. Layer **US2** (T018–T023) for real mining → demo end-to-end without manual DB seeding.  
4. Add **US3** + **US4**, then Polish.

### Incremental delivery

Each story ends with its **Independent Test** narrative from `spec.md`, ensuring reviewers can merge vertical slices without waiting for later stories (except where explicitly noted).

### Format validation

- Total tasks: **32**  
- Count by story label: **US1** ×4, **US2** ×6, **US3** ×4, **US4** ×2; foundation/polish/tasks without story markers for shared infrastructure/documentation.  
- Every task begins with `- [ ]`, sequential `T###` identifiers, descriptive text referencing concrete file paths inside `github.com/devspecs-com/devspecs-cli`.

---

## Notes

- Tests included because PLAN + spec success criteria mandate automated coverage (hooks idempotency, mining JSON regression, deterministic scoring). Adjust scope only if stakeholder explicitly relaxes QA expectations.
- Respect performance caps documented in PLAN when implementing `--all` mining (surface warnings via JSON per contracts when truncating commits/files).
- When uncertain where tests live (`internal/commands`, `internal/mining`, integration harness), inspect existing `_test.go` placement before adding duplicates.
