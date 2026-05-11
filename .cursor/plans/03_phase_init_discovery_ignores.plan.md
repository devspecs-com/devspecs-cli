---
name: Phase 3 — init discovery + ignore stack
overview: internal/discover; .gitignore + .git/info/exclude + repo-root .aiignore; capped traversal; ds init flags; no wizard prompts; narrow docs/ heuristics; defer .cursorignore.
todos:
  - id: p3-ignore-implementation
    content: Choose/implement gitignore-style matcher (.gitignore, .git/info/exclude, repo-root .aiignore); document in README.
    status: completed
  - id: p3-ignore-tests
    content: "internal/ignore (or equivalent): API + unit tests for ignore stack and edge cases."
    status: completed
  - id: p3-discover
    content: "internal/discover: capped walk, confidence heuristics, high vs maybe lists; never return ignored paths; tests."
    status: completed
  - id: p3-walk-plumbing
    content: Plumb matcher into markdown + ADR directory walks; shared regression tests.
    status: completed
  - id: p3-ds-init
    content: "ds init: git root, --yes/--non-interactive/--no-detect, merge YAML, print suggestions only; preserve --force/--hooks."
    status: completed
  - id: p3-docs-policy
    content: Plain docs/ high-confidence gating; test sparse docs/ not auto-injected.
    status: completed
  - id: p3-verbose
    content: "Optional: ds scan --verbose one-line ignore notice (or defer + document)."
    status: completed
  - id: p3-regression
    content: Init/discover/ignore integration tests; existing tests green.
    status: completed
  - id: p3-commits
    content: Incremental commits per index discipline.
    status: completed
isProject: false
---

# Phase 3: Init discovery + ignore stack

**Goal**: **Init** feels smart; **scan** stays “read config.” Ship after phases **1–2** to limit blast radius.

## Tasks

- [x] **Dependency / implementation**: choose or implement gitignore-style matcher (`.gitignore`, `.git/info/exclude`, repo-root `.aiignore`); document semantics in README.
- [x] **`internal/ignore` (or under discover)**: API `ShouldSkip(repoRoot, relPath, isDir)` or equivalent; unit tests for ignore + exclude + aiignore + edge cases.
- [x] **`internal/discover`**: capped directory walk, confidence heuristics, “high vs maybe” candidate lists; **never** return ignored paths; tests with temp dirs.
- [x] **Markdown + ADR walks**: plumb matcher into discovery walks ([`markdown`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/markdown/markdown.go), [`adr`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/adapters/adr/adr.go)); shared tests with discover matcher.
- [x] **`ds init`**: resolve repo root (Git root when applicable); add `--yes` / `--non-interactive` / `--no-detect` (document); merge detected high-confidence paths into YAML; print suggestions only (**no prompts**); preserve `--force` / `--hooks`.
- [x] **Plain `docs/` policy**: implement high-confidence gating before auto-adding `docs/`; test sparse `docs/` tree not auto-injected.
- [x] **Optional**: `ds scan --verbose` one-line ignore notice (if not deferred).
- [x] **Tests**: init integration (non-TTY), discover caps, ignore regressions; existing init/scan tests green.
- [x] **Commits**: incremental per index discipline.

## Ignore stack (v0.1)

**Priority order** (merged matcher, tests for interaction):

1. `.gitignore` (standard stack as implemented by chosen library from repo root)
2. `.git/info/exclude`
3. **Repo-root `.aiignore` only** — gitignore-like globs where practical

**Applies to**

- **Init-time discovery** traversal
- **Configured-path recursive walks** (markdown + ADR adapters) — same matcher as discover (“discovery and configured scans respect ignores”)

**Defer as first-class v0.1**: `.cursorignore` / `.cursor/ignore` until usage is validated.

**Out of scope v0.1 unless trivial**

- `--include-ignored` / config overrides
- Nested `.aiignore` in subdirectories
- Document **negation** if the parser supports it

**UX**

- Optional `ds scan --verbose`: one line that `.gitignore` + `.aiignore` apply (repo root)
- Init discovery: optional “N paths skipped (ignored)”

Cross-links: [00_index_devspecs_discovery_format.plan.md](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md).

## `internal/discover`

- Capped traversal (max dirs, timeout, sampling — tune in implementation).
- High-confidence roots: e.g. `openspec/changes`, `.cursor/plans`, Spec Kit shapes, `_bmad-output`, …
- **Plain `docs/`**: do **not** auto-merge into config unless **high-confidence** (e.g. density of `*.spec.md`, `*.plan.md`, `*.prd.md`, …). Prefer narrower defaults/suggestions: `docs/specs`, `docs/plans`, `docs/design`, `docs/technical`, `docs/adr`, …
- **Ignored paths** must never appear as proposed candidates.

## `ds init` — **no interactive prompts (v0.1)**

TTY and non-TTY behave the same:

1. Run discovery unless `--no-detect`.
2. **Merge high-confidence** detected paths into [`.devspecs/config.yaml`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/config/config.go) (union with sensible defaults).
3. Print **low-confidence** paths as **suggestions only** (`ds config add-source …` / YAML snippets).
4. Flags: `--yes` / `--non-interactive` (document CI intent; behavior matches default), `--no-detect`, `--force`, optional `--hooks` unchanged.
5. Repo root: prefer **Git root** from [`internal/repo`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/repo) when in a git worktree; document.

**Do not** ship multi-select wizards in v0.1.

## Touchpoints

- [`internal/commands/init.go`](c:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/internal/commands/init.go)
- New `internal/discover` + shared ignore helper used by init and (same release) markdown/ADR walks

## Auditable success criteria (phase 3)

- [x] **Ignore tests**: fixture repo (or temp dir) with `.gitignore` listing `ignored/` — `discover` / walk does not recurse into or list that dir as a candidate; same for **repo-root `.aiignore`** with a distinct path (two small tests).
- [x] **`.git/info/exclude`**: at least one test or documented manual check that an excluded path behaves like `.gitignore` per chosen implementation.
- [x] **Markdown/ADR walks** use the same matcher as discover (shared function or package — assert via test that a path ignored in one is ignored in the other).
- [x] **`ds init`**: no code path calls interactive prompt libraries; running with stdin closed does not hang (CI-friendly).
- [x] **`ds init --no-detect`**: written config matches prior “defaults only” behavior within documented expectations.
- [x] **Plain `docs/`** without plan/spec-like files: not auto-added to `sources` (test with sparse `docs/` tree).
- [x] **`--force` / hooks**: existing behavior still passes prior tests.
- [x] Incremental commits per index discipline.

## Implementation note

Follow [implementation discipline](file:///C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/.cursor/plans/00_index_devspecs_discovery_format.plan.md#implementation-discipline-all-phases).
