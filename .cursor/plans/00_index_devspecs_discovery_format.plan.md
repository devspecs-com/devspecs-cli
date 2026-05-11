---
name: DevSpecs CLI — discovery & format (index)
overview: Phased rollout — format_profile + scan UX first; init discovery + ignore stack second; avoid taxonomy epic. See linked phase files.
isProject: false
---

# DevSpecs CLI: discovery, ignores, format — plan index

**Principle**: *Init owns discovery; scan owns configured indexing.* *Smart during setup, boring during execution.*

Ship in **four reviewable phases** (GPT/Composer scope split). Deep reference material (adapter rationale, bundle trade-offs) stays in [phase 01](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/01_phase_format_profile_and_persistence.plan.md); this index stays short.

## Implementation discipline (all phases)

The implementing agent should:

- **Commit incrementally** — small, coherent commits as work progresses (e.g. migration alone, then store write path, then adapter + tests), not one lump-sum commit at the end. Each commit should leave the repo **buildable** and **`go test ./...` green** for packages touched.
- **TDD / test-led where practical** — add or extend a failing test that captures the behavior (unit tests for `internal/format` and ignore matching, store/migration tests, CLI/golden tests) before or in tight alternation with implementation; avoid large untested surface.
- **Clean code** — match existing project conventions; minimal scope per change; no drive-by refactors; keep ignore/matcher and format normalization in **small, testable** packages.

**Cursor To-dos / plan UI**: Each phase `.plan.md` includes a **`todos` list in YAML frontmatter** (`id`, `content`, `status`: `pending` \| `in_progress` \| `completed` \| `error` as you adopt). That is what Cursor surfaces in the **To-dos** panel when the file is treated as a plan—body **## Tasks** alone does not sync there. After editing tasks, keep **frontmatter `todos` and ## Tasks aligned**.

Each phase file also has **## Auditable success criteria** for verification. Work in order; commit incrementally.

| Phase | File | Ship when |
|-------|------|-----------|
| **1** — Format profile + persistence | [01_phase_format_profile_and_persistence.plan.md](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/01_phase_format_profile_and_persistence.plan.md) | First (lowest risk) |
| **2** — Scan output | [02_phase_scan_output.plan.md](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/02_phase_scan_output.plan.md) | After phase 1 |
| **3** — Init discovery + ignore stack | [03_phase_init_discovery_ignores.plan.md](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/03_phase_init_discovery_ignores.plan.md) | After phase 2 |
| **4** — Empty scan hints | [04_phase_empty_scan_hints.plan.md](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/04_phase_empty_scan_hints.plan.md) | After phase 3 (or parallel if discover reused) |

### Contract vs. shortcuts
- The **plan is the spec** an auditor can trace: tasks, success criteria, and stated behavior (including JSON shape, flags, and naming).
- Agent will **not** ship an unplanned shortcut and then **edit the plan afterward** so the audit still “passes.” That would hide drift.
- If reality **must** differ from the plan (API constraint, backward compatibility, bugfix), I’ll either **get the plan updated first** (with you) or **implement what the plan says** and call out the gap explicitly—not silently reconcile the doc to the code.

### What Agent *will* do while implementing
- **Move task / checklist / YAML todo status as work progresses** (in progress → done), not only at the end.
- **Add implementation notes** to the plan when they help auditors and future you: e.g. “wire JSON key is `Found` because `scan.Result` uses std `encoding/json` and existing clients rely on it” — that’s **traceable context**, not rewriting success criteria to match a shortcut.
- If something in the plan is **ambiguous** (e.g. doc says `found`, code has always emitted `Found`), Agent will add a short **“Auditor / wire format”** note so comparison is explicit, rather than only changing the example so the audit “looks green.”

Treat **plan edits during work** as normal for **status + honest notes**; Treat **plan edits** as **wrong** when they’re used to **mask unplanned implementation drift**.

## What ships in each phase (summary)

1. **`internal/format`**, `format_profile` on sources, **`extracted_json` persistence**, stop path/tool slugs as tags, **`layout_group`** (optional, for future UX) — see phase 01.
2. Human **`ds scan`** labels (“Planning docs”, “OpenSpec”, “ADRs”), format breakdown, **`ds scan --json`** adds `sources_breakdown` while **keeping** flat `found` — see phase 02.
3. **`internal/discover`**, ignore stack (`.gitignore`, `.git/info/exclude`, repo-root `.aiignore`), capped traversal, **`ds init`** flags (`--yes`, `--non-interactive`, `--no-detect`), **no interactive prompts** (print + auto-merge high-confidence); **defer `.cursorignore`** until validated — see phase 03.
4. When scan finds **zero** artifacts, bounded hints + `ds config add-source` examples — see phase 04.

## v0.1 rules (cross-phase)

- **`ds scan`** remains **deterministic** and **config-driven** after init; **never prompts**; **`ds scan --json`** never prompts and stays **backward compatible** (retain `found` map).
- **`sources.source_type`** stays `markdown` | `openspec` | `adr`; human output uses clearer labels where needed.
- **Tags** = user/domain (`tags`/`labels` frontmatter, manual); **not** path-derived tool slugs — use **`format_profile`**.
- **Ignore priority** (implementation order): `.gitignore` → `.git/info/exclude` → **repo-root `.aiignore`** only (v0.1); optional later: `.cursorignore` / `.cursor/ignore` — **not** first-class until usage confirmed.
- **`.aiignore` (v0.1 semantics)**: repo-root file only; gitignore-like globs where practical; applies to **init discovery** and **recursive markdown/ADR walks** (same stack as discover). **No** `--include-ignored` / config override flags until needed. Nested `.aiignore`: **out of scope** for v0.1 unless trivial with chosen library.
- **Init UX**: **No multi-select wizard** — TTY and non-TTY behave the same: **detect** → **write high-confidence paths automatically** (unless `--no-detect`) → **print** “maybe” candidates as suggestions (no blocking prompts). Reduces test/UX churn.
- **Broad `docs/`**: do **not** auto-add plain `docs/` from discovery unless **high-confidence** (e.g. density of `*.plan.md`, `*.spec.md`, …). Prefer narrow paths: `docs/specs`, `docs/plans`, `docs/design`, etc. — details in phase 03.
- **User-facing copy**: keep **simple** (“Planning docs / OpenSpec / ADRs”). Full **four-axis** taxonomy (`source_type`, `kind`, `format_profile`, `tags`) lives in **docs**, not hero README wall-of-taxonomy.
- **Risk**: taxonomy overinvestment — prioritize **`ds scan → resume → context`** clarity over ontology depth.

## Global acceptance criteria (auditable, after all phases)

Verifier can confirm each item concretely:

- [ ] `go test ./...` passes at repo root on a clean clone after all phases merge.
- [ ] `ds scan` with fixed config on a fixture repo yields the same artifact counts when run twice (determinism).
- [ ] `ds scan` never reads stdin / blocks for user input (no prompts).
- [ ] `ds scan --json` output includes legacy **`found`** map with same keys as today (`markdown`, `openspec`, `adr`) and parses as JSON without error; includes **`sources_breakdown`** (phase 2).
- [ ] Init discovery (phase 3): temporary repo with `.gitignore` excluding `private/` does not list `private/` in suggested sources; repo-root `.aiignore` entries are honored the same way (integration or unit test on matcher).
- [ ] Path-inferred tool labels (e.g. `.cursor/plans`) appear in **`sources.format_profile`** (or equivalent), not as **`artifact_tags`**, for newly scanned rows; frontmatter `tags:` still appear as tags (test with fixture markdown).
- [ ] After scan, `artifact_revisions.extracted_json` is non-empty for a fixture artifact where `Extracted` is populated (SQL or API test).
- [ ] Zero-artifact configured scan exits **0** and prints a **bounded** hint list (max N lines/paths); `ds scan --json` includes **`hints`** when phase 4 specifies it.

## Supersedes

Monolithic plan content is split across phase files. Legacy filename: `init_scan_discovery_format.plan.md` → **pointer only** to this index.

## Kleio

Log checkpoint/decision after each phase that changes schema, scan behavior, or init contract.
