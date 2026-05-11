---
name: Phase 1 — format profile + persistence
overview: internal/format, format_profile column, extracted_json, stop tag pollution, optional layout_group for future grouping UX.
todos:
  - id: p1-internal-format
    content: "internal/format: closed vocabulary, Normalize/default generic, table-driven unit tests."
    status: completed
  - id: p1-schema-store
    content: "Schema + store: migration for sources.format_profile (and layout_group if in scope); wire Insert/Update; migration test."
    status: completed
  - id: p1-adapter-types
    content: "Types: FormatProfile (+ optional LayoutGroup) on adapters.Candidate, Artifact, Source; thread through scan to DB."
    status: completed
  - id: p1-extracted-json
    content: Persist artifact_revisions.extracted_json on insert and content-change update in internal/scan/scan.go; test with fixture.
    status: completed
  - id: p1-markdown-adapter
    content: "Markdown adapter: format_profile from path + frontmatter; stop path-derived tool strings as tags; keep user tags/labels."
    status: completed
  - id: p1-openspec-adr
    content: "OpenSpec + ADR adapters: set format_profile openspec/adr; populate Source fields."
    status: completed
  - id: p1-layout-group
    content: "LayoutGroup (if in PR): Speckit/BMAD heuristics + test; else defer and note in PR / success criteria."
    status: completed
  - id: p1-regression
    content: "Regression: go test ./...; update markdown, openspec, adr, store, scan tests."
    status: completed
  - id: p1-commits
    content: Incremental commits per index implementation discipline.
    status: completed
isProject: false
---

# Phase 1: Format profile and persistence

**Goal**: Fix data model clarity and a real gap (`Extracted` not persisted) before onboarding/discovery work. Lower blast radius than full init rewrite.

## Tasks

- [x] **`internal/format`**: closed vocabulary constants, `Normalize` / default `generic`, unit tests (table-driven where useful).
- [x] **Schema + store**: migration for `sources.format_profile` (and `layout_group` if in scope); `Insert`/`Update` paths set columns; migration test.
- [x] **Types**: add `FormatProfile` (and optional `LayoutGroup`) on [`adapters.Candidate`, `Artifact`, `Source`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/adapters.go); thread through scan → DB.
- [x] **`artifact_revisions.extracted_json`**: serialize `Artifact.Extracted` on **insert** and **content-change update** in [`internal/scan/scan.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go); test with fixture parse.
- [x] **Markdown adapter**: compute `format_profile` from path + frontmatter; **remove** path-derived tool strings from tag emission; keep user `tags`/`labels` from frontmatter as tags.
- [x] **OpenSpec / ADR adapters**: set `format_profile` to `openspec` / `adr`; populate `Source` fields.
- [x] **LayoutGroup** (if in this PR): Speckit/BMAD heuristics + test; else explicitly defer in PR description and uncheck layout criterion in success list.
- [x] **Regression**: `go test ./...`; fix/update [`markdown`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/markdown), openspec, adr, store, scan tests.
- [x] **Commits**: incremental, logical steps per [index discipline](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md#implementation-discipline-all-phases).

## Four axes (reference for implementers)

| Axis | Storage / usage |
|------|------------------|
| `source_type` | Ingestion pipeline — `markdown`, `openspec`, `adr` — unchanged in config/DB |
| `kind` | Semantic artifact type — `artifacts.kind` |
| `format_profile` | System convention / layout — **new** `sources.format_profile` (closed vocabulary) |
| `tags` | User/domain — frontmatter `tags`/`labels`, manual only; **not** path tool slugs |

Example shape (conceptual):

```txt
source_type: markdown
kind: plan
format_profile: cursor_plan
tags: ["billing", "mvp"]
```

## Implementation steps

1. Add [`internal/format`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli) (or equivalent): constants, normalization, `generic` fallback; optional `--verbose` weak-inference warning later (phase 2).
2. Extend [`Candidate` / `Artifact` / `Source`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/adapters.go): **`FormatProfile`** string; **`LayoutGroup` optional** — repo-relative key for “same feature folder” (e.g. `specs/001-discover-related-specs` for Spec Kit) so future grouping/bundle UX does not require another migration. May live on `Source` or inside `Extracted` if you want zero schema change — **prefer column on `sources` if cheap**, else derive from path prefix in phase 1 and add column in a follow-up migration.
3. **DB migration**: `sources.format_profile TEXT` nullable; optional `sources.layout_group TEXT` nullable (Recommended if one migration is acceptable).
4. **Persist [`artifact_revisions.extracted_json`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/store/schema.sql)** on insert and update paths in [`internal/scan/scan.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go) — JSON serialize `Artifact.Extracted` (minimum fix for `show` / `context` / MCP).
5. **Markdown adapter**: set `format_profile` from path + frontmatter (`generator` / `tool` / `source`); **stop** emitting path-derived tooling strings as **`artifact_tags`** — see [`pathGeneratorHints`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/markdown/markdown.go) / [`replaceTags`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/scan/scan.go).
6. **OpenSpec / ADR adapters**: set fixed `format_profile` (`openspec`, `adr`) for consistency in reporting and filters.
7. **`LayoutGroup`**: for `speckit`-shaped paths, set to feature directory (e.g. parent of `spec.md`); for BMAD, optional group key for `planning-artifacts` dir — **no CLI surface required** in phase 1.
8. **Tests**: update markdown/openspec/adr tests; migration test; golden tests for tags vs format.

## Adapter rationale (short; do not expand in code reviews)

- **Separate adapter** when **identity / discovery / parsing contract** differs materially (OpenSpec change folder, ADR conventions).
- **Cursor / BMAD / Spec Kit file trees**: remain **`markdown` ingestion** + **`format_profile`** + optional **`layout_group`**; file-per-artifact v0 default.

## Bundle (composite) artifact — deferred

One row per feature / BMAD run is a **cleaner mental model** but hits identity, revision, CLI granularity, and FTS trade-offs. **v0 default stays file-per-row**; **`layout_group`** preserves enough to group later. Longer bundle trade-off write-up available in git history of `init_scan_discovery_format.plan.md` if needed; avoid blocking phase 1.

## Out of phase 1

- Richer `ds scan` human/JSON breakdown — **phase 2**.
- Ignore stack, `ds init` — **phase 3**.
- Empty-scan hints — **phase 4**.

## Auditable success criteria (phase 1)

Verifier checks each item:

- [x] Schema migration applies cleanly from previous version; **`go test ./...`** passes; migration test or manual upgrade path documented if needed.
- [x] `sources` rows written by scan include **`format_profile`** (non-null for normal markdown/openspec/adr cases; unknown → documented default e.g. `generic`).
- [x] If **`layout_group`** was scoped in this phase: set for at least one Speckit-style fixture path in tests; otherwise criterion N/A with comment in PR.
- [x] **`artifact_revisions.extracted_json`**: for a scan that sets `Artifact.Extracted`, persisted revision row contains valid JSON matching that map (assert in store or scan integration test).
- [x] Fixture markdown under `.cursor/plans/` (or equivalent): after scan, **`artifact_tags`** does **not** contain prior path-derived tool slug (e.g. `cursor`) **unless** it is a user frontmatter tag; **`format_profile`** reflects `cursor_plan` (or chosen slug).
- [x] OpenSpec + ADR scans set **`format_profile`** to `openspec` / `adr` respectively (test).
- [x] Config **`type: markdown`**, CLI **`--source markdown`**, and DB **`source_type`** values are unchanged.
- [x] Incremental commits: PR description or git history shows **logical commit steps** (not a single opaque blob).

## Implementation note

Follow **index** [implementation discipline](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md#implementation-discipline-all-phases).
