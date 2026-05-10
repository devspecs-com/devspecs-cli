---
stepsCompleted:
  - step-direct-authoring-complete
inputDocuments: []
workflowType: prd
prdTitle: "devspecs-cli: Probabilistic related specs (v1)"
prdAudience: "Engineers using ds locally to index specs/plans and resume work"
---

# Product Requirements Document — devspecs-cli: Probabilistic related specs (v1)

**Author:** Brenn  
**Date:** 2026-05-10  
**Status:** Draft for implementation alignment

## Executive summary

**Goal:** Make it easy for developers who use `ds` to see which DevSpecs are *likely* relevant to the file they are editing or the branch they are on, and to keep that association fresh with minimal manual bookkeeping—without claiming causal blame, without a background daemon, and without embeddings or LLM similarity for this release.

**Problem today:** Linking day-to-day coding work to the right markdown/OpenSpec/ADR-style artifacts is manual. After indexing, it remains hard to answer: “Given this path or this branch, what specs should I read or update next?”

**Solution outline (v1):** Three commands—**related** (query), **workon** (declare intent), **mine** (gather evidence)—backed by durable, explainable evidence rows that the CLI aggregates into ranked results.

---

## Goals and non-goals

### Goals

1. **Explainable relatedness:** Every suggestion must surface human-readable *reasons* (evidence lines), not only a score.
2. **Probabilistic framing:** Copy and behavior must communicate *likelihood*, not authorship or blame.
3. **Local-first workflows:** Everything runs in the developer’s repo/worktree using git, indexed specs, todos, and session state—suitable for solo developers and small teams.
4. **Composable with existing `ds`:** Integrate with scan/list/show/resume patterns; preserve rebuild/migration expectations for store upgrades.
5. **Automation-friendly:** Optional git hooks keep evidence reasonably current without requiring a long-running process.

### Non-goals (explicit for this slice)

- Pull-request provider integration (GitHub/GitLab APIs, review comment mining).
- A daemon or file watcher service.
- Vector embeddings or LLM-based semantic similarity as an input signal.
- Replacing formal traceability; v1 does not certify that a spec “caused” a code change.

---

## Users and beneficiaries

| Segment | Needs |
|--------|--------|
| **Solo maintainer** | Quickly find specs tied to the module they touched; reduce context re-discovery after time away. |
| **Small team** | Shared conventions (markdown specs next to code); predictable `ds` commands for onboarding. |
| **Agent-assisted workflow** | Stable `--json` shapes and evidence types for tooling and tests. |

**Primary user:** An engineer already running `ds scan` (or hooks) to index DevSpecs, who wants ranked hints keyed off a **repo-relative path** or **current branch context**.

---

## User scenarios

### S1 — “What specs matter for this file?”

**Trigger:** Developer opens or edits a source or doc file.  
**Action:** Run related query for that path (repo-relative or resolvable to one).  
**Outcome:** CLI prints ranked DevSpecs with **high** and **medium** confidence by default; optional flag includes **low**. Each artifact shows **evidence lines** (type + short explanation), not a bare numeric rank.

**Acceptance cues:** Results are stable for the same store state; omitting low bucket changes output as specified; JSON mode includes artifact id, confidence, bucket, and evidence list.

### S2 — “This branch is about artifact X”

**Trigger:** Developer starts work that centers on a specific DevSpec (story, change, ADR).  
**Action:** Declare **workon** with that artifact id (full, short, or prefix per existing resolution rules).  
**Outcome:** Current repo root, worktree root, branch, HEAD, and artifact association are stored; any prior open session for the same triple is closed. Clearing workon ends the session without deleting historical evidence.

**Acceptance cues:** Idempotent behavior for repeat declarations; clear user-facing messages; show command reports active association or none.

### S3 — “Refresh evidence after I commit or move branches”

**Trigger:** Git history or working tree changes; developer wants the store to reflect new signals.  
**Action:** Run **mine** (full or recent scope); optional hooks run quiet variants after commit/merge/checkout/rewrite.  
**Outcome:** Evidence rows are upserted (no duplicate rows for the same logical evidence key); additive confidence per artifact/file is computed at **related** time and capped.

**Acceptance cues:** `--recent` suitable for hooks (bounded work); `--all` remains conservative on large repos; `--quiet` suppresses chatter in hook contexts.

### S4 — “CI/agent consumes relatedness”

**Trigger:** Automation needs ranked lists.  
**Action:** Call **related** or **mine** with `--json`.  
**Outcome:** Stable field names suitable for golden tests; bucket counts available where specified for mine summary output.

---

## Functional requirements

### FR1 — Command surface

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR1.1 | CLI provides `ds related <file>` with default output showing **high** and **medium** buckets only | Manual or automated command test; output lacks low bucket lines unless flag set |
| FR1.2 | `ds related` supports `--all` to include **low** bucket | Compare output with and without flag |
| FR1.3 | `ds related` supports `--json` with artifact identifiers, confidence, bucket, and evidence entries | Parse JSON schema in test; stable keys |
| FR1.4 | `ds workon <id>` resolves artifact id (full, short, prefix per product conventions), associates repo/worktree/branch/HEAD, ends prior open session for same scope | Command + store integration test |
| FR1.5 | `ds workon` prints active association or none | Command test |
| FR1.6 | `ds workon --clear` ends active session for current scope | Command test |
| FR1.7 | `ds mine` defaults to sensible current-repo scope; supports `--recent`, `--all`, `--json` | Command test; hook uses `--recent` |
| FR1.8 | `ds mine` supports `--quiet` for non-interactive/noise-sensitive contexts | Assert stderr/stdout expectations in test |

### FR2 — Evidence model (behavioral)

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR2.1 | Evidence is stored per **artifact** and **normalized file path** with typed reasons; multiple rows may exist for one pair | Store test |
| FR2.2 | Paths are normalized to repo-relative, forward-slash form consistent with existing adapters | Unit test for normalization |
| FR2.3 | Supported evidence **types** for v1: `manual`, `workon_branch`, `explicit_commit_ref`, `same_commit`, `branch_name_match`, `commit_message_match`, `spec_mentions_file`, `todo_mentions_file`, `same_directory` | Mine produces each type in fixture scenarios |
| FR2.4 | **Confidence** per evidence type uses the agreed constants (including manual = 1.00, workon_branch = 0.75, explicit commit ref to DevSpec id = 0.50, same commit spec+code = 0.45, branch slug match = 0.35, spec body path/name = 0.30, commit message token = 0.20, same directory = 0.15, todo mention = 0.10) | Golden or table-driven unit test |
| FR2.5 | **Aggregation:** Summing confidence for the same artifact/file is **additive** with **hard cap 1.0** | Pure function unit test |
| FR2.6 | **Buckets:** high ≥ 0.75, medium ≥ 0.45, low ≥ 0.20; related output maps aggregated scores to these labels | Boundary tests |

### FR3 — Mining behavior

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR3.1 | Mine inspects git signals: merge-base with default branch when available; changed files in relevant commit ranges; commit messages for full and short artifact id patterns | Git fixture tests |
| FR3.2 | Mine inspects spec bodies and todo text for path/name mentions | Fixture: spec mentions file |
| FR3.3 | When a **workon** session is active, mine emits `workon_branch` evidence for files touched in the mined commit set per agreed rules | Fixture: active session |
| FR3.4 | Mine avoids unbounded cost on huge repos for `--all` (documented caps on commits/files or equivalent guardrails) | Test or benchmark threshold; documented limits |

### FR4 — Scan / revision metadata

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR4.1 | When scan records a new artifact revision in a git checkout, the **current HEAD commit** is stored on that revision record (field populated on insert/update paths used by scan) | Scan or store regression test |

### FR5 — Durability and schema

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR5.1 | Store version increments to **v4**; fresh DBs create tables for **file-to-artifact evidence** and **work sessions** with indexes suitable for lookup by file and artifact | Schema/store tests |
| FR5.2 | Evidence upsert uses a uniqueness rule so repeated mining **updates** observation timestamps (and confidence if applicable) instead of duplicating logical rows | Store test |
| FR5.3 | Old databases follow existing product behavior for rebuild-required upgrades (no silent partial migration unless separately specified) | Documented; integration expectation |

### FR6 — Hooks

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR6.1 | `ds init --hooks` installs or appends best-effort hook snippets: post-commit runs quiet scan (if-changed) + quiet recent mine; post-checkout runs quiet scan; post-merge and post-rewrite run quiet scan + quiet recent mine | Init/freshness tests |
| FR6.2 | Hook installation remains **idempotent** if init runs multiple times | Double-init test |
| FR6.3 | Hooks continue to use fail-open pattern (`|| true` or equivalent) consistent with existing trust model | Static inspection of generated scripts |

### FR7 — Explanations and messaging

| ID | Requirement | Verification |
|----|-------------|--------------|
| FR7.1 | Related output never shows only a score; each listed artifact includes at least one evidence line | Snapshot/cli test |
| FR7.2 | User-facing strings avoid blame language; relate to “likely,” “evidence,” “association” | Copy review |

---

## Success metrics

| Metric | Definition | Target |
|--------|------------|--------|
| **Time-to-context** | Median time for a developer to identify a relevant spec for a touched file (self-reported or lab) | Meaningful reduction vs. manual search in pilot |
| **Coverage of signals** | Mine produces non-empty related results in golden scenarios (same commit, branch slug, workon, mentions) | 100% pass on agreed fixture set |
| **Explainability** | Users can state *why* a spec appeared without reading code | Qualitative + structured evidence lines always present |
| **Automation stability** | JSON outputs pass golden tests across patch releases | No unexpected key removals within major version |
| **Operational friction** | Hook-enabled repos show no noticeable commit slowdown on median projects | `--recent` completes within agreed time budget on reference repo sizes |

---

## Risks and mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **False positives** | Users distrust suggestions | Probabilistic messaging; evidence breakdown; conservative defaults (hide low bucket) |
| **False negatives** | Missed relevant specs | Encourage **workon**; manual evidence type at max weight; hooks keep mine fresh |
| **Performance on large histories** | `mine --all` too slow | Hard caps; prefer `--recent` in hooks; document guidance |
| **Worktree edge cases** | Wrong association if worktree root differs | v1 may equate worktree to repo root; document limitation or follow-up |
| **Store migration annoyance** | v4 rebuild friction | Align with existing rebuild story; clear release notes |
| **Over-interpretation of git correlation** | “Related” read as “caused” | UX copy; training in README; no `blame` naming in v1 |

---

## Dependencies and assumptions

- Developers already index DevSpecs with `ds scan` (or automation doing the same).
- Git metadata is available locally (merge-base, log, changed files).
- Artifact identifiers in commit messages and specs follow patterns mine can detect (existing id formats).
- Generic `ds link` remains for external URLs; file attribution for relatedness uses **dedicated** file-link evidence storage (not overloaded link table semantics).

---

## Open questions (non-blocking for v1)

- Exact caps for `--all` mine (max commits, max files) tuned after dogfooding.
- Whether future worktree-aware detection is required before broader release vs. documented limitation.

---

## Document control

This PRD is written as a **standalone** product specification for devspecs-cli behavior. Implementation details such as SQL DDL, package layout, and specific test file names belong in the technical architecture and sprint plans.
