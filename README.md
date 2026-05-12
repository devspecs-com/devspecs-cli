# DevSpecs CLI

> Local-first CLI for indexing specs, plans, ADRs, and agent-ready engineering context.

## Why DevSpecs?

AI-assisted development makes it easy to create plans, specs, ADRs, task lists, and design notes — but hard to keep track of them later.

A feature can quickly produce an OpenSpec proposal, a Cursor plan, a markdown checklist, a PR description, and a follow-up design note. Those artifacts are useful, but they often stay scattered across repo folders, editor state, and ad-hoc files. When you come back later, it is not always obvious what exists, which plan is still active, what todos remain, or what context to hand to the next coding session.

DevSpecs is a local-first CLI that indexes the planning/specification artifacts you already have and gives them stable references.

It is useful when you want to:

- see what specs, plans, ADRs, and design docs exist in a repo
- continue work from active or stale planning artifacts
- resolve a short ID back to the original source file
- extract todos from markdown checklists without creating a task board
- export clean context for Cursor, Claude Code, Codex, or another coding agent
- reference implementation intent from PRs, issues, commits, or future notes

DevSpecs does **not** replace Git, markdown, OpenSpec, ADRs, GitHub, Linear, or your editor. Keep writing specs where they already belong. DevSpecs adds a lightweight local index over them so humans and agents can find, reference, and reuse the right context.

## What it does

- Scans **OpenSpec** changes, **ADR** paths, and **markdown** plans/specs (including common agent layouts such as `.cursor/plans`, BMAD `_bmad-output`, Spec Kit `specs/…/spec.md`).
- Assigns **stable full IDs** and **short IDs** for everyday CLI use.
- **Extracts markdown checklist todos** and **acceptance / success / OKR criteria** (under matching headings), stored per artifact revision (source files stay authoritative).
- Surfaces **in progress**, **recently settled**, and **stale** artifacts with **`ds resume`**.
- Exports **agent-ready context** with **`ds context`**.
- Keeps everything **local** in a SQLite index under your home directory (override with **`DEVSPECS_HOME`**).

## Install

### Quick install (recommended)

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.ps1 | iex
```

### Homebrew (macOS / Linux)

```bash
brew install devspecs-com/tap/devspecs
```

The Homebrew formula is named `devspecs`; the binary is **`ds`**.

### Scoop (Windows)

```powershell
scoop bucket add devspecs https://github.com/devspecs-com/scoop-bucket
scoop install devspecs
```

### Go install

```bash
go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest
```

### Manual download

Download the latest release from [GitHub Releases](https://github.com/devspecs-com/devspecs-cli/releases).

## Quick start

From a repository root:

```bash
ds init
ds scan
ds resume
ds list
```

Example (illustrative — IDs and titles depend on your repo):

```text
$ ds list
  abcdef01  plan   -            implementing  Resize hook rollout
  23456789  decision  adr      proposed      Auth middleware ADR

$ ds show abcdef01
  Title: Resize hook rollout
  Kind: plan   Status: implementing
  Short ID: abcdef01
  ...
```

Fast paths after indexing:

```bash
ds find auth                 # search indexed text
ds todos                     # checklist items across artifacts
ds criteria                  # acceptance / success / OKR checklist criteria
ds context abcdef01          # paste-ready context for an agent
ds config show               # effective discovery paths
```

## Core workflow

1. **`ds init`** — Creates the global index location `~/.devspecs` (overridable) and repo **`.devspecs/config.yaml`**. In a Git worktree, config is written at the **repository root** (not necessarily your current working directory). In an interactive terminal (stdin and stdout are both TTYs), init runs a **workflow profile** picker first (merge paths and rules for common layouts; **Custom markdown paths** lets you list folders and then **map glob patterns to kinds** interactively), then **layout detection** merges additional paths into the YAML unless you pass **`--no-detect`**. Use **`--yes`** or **`--non-interactive`** to skip the profile picker (CI and scripts).
2. **`ds scan`** — Walks adapters and upserts artifacts, revisions, sources, todos, criteria, and tags.
3. **`ds list`** / **`ds find`** — Browse or search what was indexed.
4. **`ds show <id>`** — Full detail; accepts full ID, **short ID**, or prefix.
5. **`ds todos`** / **`ds criteria`** / **`ds resume`** — Triage checklist items, auditable criteria, and lifecycle-oriented “where was I?” views.
6. **`ds context <id>`** — Export a single artifact’s context for tools or agents.

### Scan summaries (`ds scan`)

Human **`ds scan`** lists **Planning docs**, **OpenSpec**, and **ADRs** — friendly labels for the internal pipelines **`markdown`**, **`openspec`**, and **`adr`** (same values in config and the database). Multiple markdown layouts (Cursor plans, Spec Kit, BMAD, generic plans) roll up under **Planning docs**; per-layout counts appear as **`formats`** (by **`format_profile`**). **`ds scan --json`** keeps the **`Found`** map and adds **`sources_breakdown`** with `source_type`, `label`, `count`, and `formats`. When **every adapter indexes zero artifacts**, human output switches to a short **recovery** message (bounded on-disk candidates when any exist, otherwise a generic **`ds config add-source markdown plans`** example); **`--json`** may include a **`hints`** array (`path`, `source_type`, `suggest_command`) when at least one candidate directory exists — if there are **no** candidates, the **`hints`** field is **omitted** (`encoding/json` **`omitempty`** on an empty slice), not emitted as `"hints": []`. **`--quiet`** suppresses that human recovery block but still exits **0**; **`--json`** output is unchanged by **`--quiet`**.

```bash
ds scan
ds list
ds show <id>
ds todos <id>
ds criteria <id>
ds context <id>
```

## Commands

Summary (see subsections and `ds <cmd> --help` for flags):

| Command | Purpose |
|---------|---------|
| `ds init` | Initialize global DB location and repo config (`--hooks`, `--yes` / `--non-interactive`, `--no-detect`, `--force`) |
| `ds scan` | Scan repo for artifacts (`--rebuild`, `--if-changed`, `--verbose`, `--json`, `--quiet`) |
| `ds resume` | In progress / recently settled / stale groupings |
| `ds list` / `ds ls` | List indexed artifacts |
| `ds find <query>` | Search indexed artifacts |
| `ds show` / `ds get <id>` | Artifact details (tags, scanned-by when set) |
| `ds resolve <id>` | Resolve ID to source path |
| `ds context <id>` | Export agent-ready context |
| `ds todos [id]` | Extracted checklist todos |
| `ds criteria [id]` | Extracted acceptance / success / OKR criteria |
| `ds config …` | `show`, `paths`, `add-source`, `set` |
| `ds tag` / `ds untag` | Manual tags (`artifact_tags`) |
| `ds capture <path>` | Capture a file as an artifact |
| `ds status <id> <s>` | Update artifact status |
| `ds link <id> <target>` | Add a link between artifacts |
| `ds version` | Print version |

Tree-style overview:

```
ds
  init                Initialize DevSpecs
  scan                Scan repository for artifacts
  list (ls)           List indexed artifacts
  show (get) <id>     Show artifact details
  find <query>        Search artifacts
  resolve <id>        Resolve ID to source path
  context <id>        Export agent-ready context
  todos [id]          List extracted todos
  criteria [id]       List extracted criteria (acceptance / success / OKR)
  resume              Lifecycle-oriented resume
  config              Show, paths, add-source, set
  tag / untag         Manage artifact tags
  capture <path>      Capture a file as an artifact
  status <id> <s>     Update artifact status
  link <id> <target>  Add a link to an artifact
  version             Show version
```

### Scripting and `--json`

Most read commands support **`--json`** for machine-readable output — useful for scripts and CI.

### Scoping filters

These flags narrow results by repo basename, tag, git branch, or scanned-by user:

**`--repo`**, **`--tag`**, **`--branch`**, **`--user`**

For **`list`** and **`find`**, you can also filter by **`--kind`** and **`--subtype`** on indexed artifacts.

They apply to **`list`**, **`find`**, **`todos`**, **`criteria`**, and **`resume`**. For **`--repo`**, pass the directory **basename** (e.g. `my-app`), not a full path.

### `ds init`

Creates **`~/.devspecs/devspecs.db`** (global index) and **`.devspecs/config.yaml`** (repo config).

```bash
ds init                      # Interactive profile picker + layout detection (TTY); non-TTY skips the picker
ds init --yes                # Skip profile picker; layout detection still runs unless --no-detect
ds init --non-interactive    # Same as --yes (CI / scripts)
ds init --no-detect          # Defaults-only YAML (no discovery merge)
ds init --force              # Overwrite existing config
ds init --hooks              # Install git post-commit hook for auto-indexing
```

Canonical **`kind`** values include **`plan`**, **`spec`**, **`requirements`**, **`design`**, **`contract`**, **`decision`**, **`markdown_artifact`**. Optional **`subtype`** distinguishes variants (for example **`openspec_change`**, **`adr`**, **`prd`**). **`ds list`** and **`ds find`** human output includes a **SUBTYPE** column; filter with **`--subtype`** (with **`--kind`**). Under **`sources`** → **`markdown`**, optional **`rules`** map path globs to **`kind`** / **`subtype`** (see `.devspecs/config.yaml`).

**Ignore stack (scan + discovery):** from the repository root, patterns are read in order from **`.gitignore`**, **`.git/info/exclude`** (when `.git` exists), then repo-root **`.aiignore`** (gitignore-like syntax, including `!` negation where the matcher supports it). **`ds scan`** applies the same rules to configured markdown and ADR directory walks. **`ds scan --verbose`** prints a one-line reminder on stderr. **`.cursorignore`** is not read.

Bare top-level **`docs/`** is not in the default markdown path list; init **merges** it only when discovery finds enough plan/spec-like files under `docs/`. Otherwise you get a short suggestion to add paths manually. Discovery caps work under `docs/` (directory and file visit limits) and caps how many sibling folders are considered under `specs/` and `openspec/changes/`.

### `ds scan`

```bash
ds scan              # Scan current directory
ds scan --path .     # Explicit path
ds scan --verbose    # Detailed output
ds scan --json       # JSON output
ds scan --if-changed # Only scan if configured source paths changed in the last commit
ds scan --rebuild    # Delete global DB, then open & scan
```

The CLI applies **bounded automatic schema migrations** when opening the global index. Use **`ds scan --rebuild`** when the database is incompatible with this CLI version or you want a clean slate (equivalent to deleting **`DEVSPECS_HOME/devspecs.db`** per error text).

If configured paths yield **no** artifacts, the CLI runs a **bounded** on-disk pass (respecting the same **`.gitignore` / exclude / `.aiignore`** rules as scanning) and prints up to a fixed number of candidate directories plus an example **`ds config add-source`** command when candidates exist; otherwise it prints a generic **`ds config add-source markdown plans`** line — still **exit 0**. On **`ds scan --json`**, the **`hints`** field follows **`omitempty`**: present only when the candidate list is non-empty.

### `ds resume`

Groups artifacts into **In Progress**, **Recently Settled** (~14 days by default), and **Stale** (non-terminal, idle ~30+ days). Supports **`--limit`**, **`--all`**, **`--no-refresh`**, short IDs, inline tags, and **`--json`** (`in_progress`, `recently_settled`, `stale`).

### `ds config`

Inspect or edit **`.devspecs/config.yaml`**: **`ds config show`**, **`paths`**, **`add-source`**, **`set`**. With no YAML yet, defaults match built-in discovery paths.

### `ds tag` / `ds untag`

Manual tags live in **`artifact_tags`**. Auto tags from frontmatter / paths refresh on scan; manual tags are preserved unless removed.

### `ds todos`

```bash
ds todos             # All todos (defaults to open)
ds todos <id>        # Todos for one artifact
ds todos --open      # Incomplete only
ds todos --done      # Completed only
ds todos --json      # JSON output
```

Honors the same **`--repo`**, **`--tag`**, **`--branch`**, **`--user`** filters as other read commands.

### `ds criteria`

Checklist lines under headings such as **Acceptance criteria**, **Success criteria** / **Success criterion**, **Auditable success** (and related phrases), **Definition of done**, or **OKR** / **Objectives and key results** are stored separately from actionable todos. Kinds in output and **`--kind`** are **`acceptance`**, **`success`**, and **`okr`**.

```bash
ds criteria              # All criteria (all repos in the index unless filtered)
ds criteria <id>         # Criteria for one artifact
ds criteria --open       # Incomplete only
ds criteria --done       # Satisfied only
ds criteria --kind success   # Only success-style criteria
ds criteria --json     # JSON array rows
```

Honors **`--repo`**, **`--tag`**, **`--branch`**, **`--user`** when listing across artifacts (same semantics as **`ds todos`**). **`--no-refresh`** skips the auto-scan freshness check.

## Supported artifact types

| Adapter | Detected paths | Kind |
|---------|---------------|------|
| OpenSpec | `openspec/changes/<id>/proposal.md` | `spec` (`subtype`: `openspec_change`) |
| ADR | `docs/adr/*.md`, `docs/adrs/*.md`, `adr/*.md`, `adrs/*.md`, `architecture/decisions/*.md` | `decision` (`subtype`: `adr`) |
| Markdown | Recursive `.md` under defaults: `specs`, `docs/specs`, `plans`, `docs/plans`, `.cursor/plans`, `docs/design`, `docs/technical`, **`_bmad-output`**, **`.specify/memory`**; plus repo-root globs `*.spec.md`, `*.plan.md`, `*.prd.md`, `*.design.md`, `*.contract.md`, `*.requirements.md` | `plan`, `spec`, `requirements`, `design`, `contract`, `markdown_artifact`, … (optional **`subtype`** e.g. **`prd`**) |

Tags may come from YAML **`tags`** / **`labels`**, directory inference, **`ds tag`**, or **path hints**: **`bmad`** / **`bmad-method`** under `_bmad-output/`; **`speckit`** for `specs/<feature>/spec.md`; **`cursor`** / **`cursor-plan`** under `.cursor/plans/`. Optional frontmatter **`generator`**, **`tool`**, **`source`** add slug tags and **`extracted.generator`**.

### Root glob vs `PLAN.md`

Repo-root globs use patterns like **`*.plan.md`**. On **case-sensitive** filesystems (typical Linux CI), that does **not** match **`PLAN.md`**. Prefer **`plans/PLAN.md`**, **`*.plan.md`-style names**, or an explicit markdown path in config.

## Configuration

`.devspecs/config.yaml`:

```yaml
version: 1

sources:
  - type: openspec
    path: openspec
  - type: adr
    paths:
      - docs/adr
      - docs/adrs
  - type: markdown
    paths:
      - specs
      - docs/specs
      - plans
      - docs/plans
      - .cursor/plans
      - docs/design
      - docs/technical
      - _bmad-output
      - .specify/memory
```

## Local index and schema

| Location | Role |
|----------|------|
| `~/.devspecs/devspecs.db` | Global SQLite index |
| `.devspecs/config.yaml` | Per-repo discovery config |
| `DEVSPECS_HOME` | Optional override for the global DevSpecs directory |

Tables include **`repos`** (optional **`scanned_by`**), **`artifacts`** (deterministic **short_id**), **`artifact_revisions`**, **`sources`**, **`links`**, **`artifact_todos`**, **`artifact_criteria`**, **`artifact_tags`**.

### Extracted todos (`artifact_todos`)

Checklist lines are re-extracted when file content changes on scan.

```sql
CREATE TABLE artifact_todos (
  id          TEXT PRIMARY KEY,
  artifact_id TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  ordinal     INTEGER NOT NULL,
  text        TEXT NOT NULL,
  done        INTEGER NOT NULL CHECK (done IN (0, 1)),
  source_file TEXT NOT NULL,
  source_line INTEGER NOT NULL,
  created_at  TEXT NOT NULL
);
```

Example **`--json`** shape:

```json
{
  "artifact_id": "ds_01JY8...",
  "revision_id": "rev_01JY8...",
  "ordinal": 3,
  "text": "Wire OpenSpec tasks parser",
  "done": false,
  "source_file": "openspec/changes/add-sso/tasks.md",
  "source_line": 12
}
```

## Reference

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Failure (includes validation errors, unknown IDs, I/O errors, and invalid arguments) |

There is no separate exit code for “user error” versus other failures today.

### Statuses

`draft`, `proposed`, `approved`, `implementing`, `implemented`, `superseded`, `rejected`, `unknown`.

### Link types

`related`, `implements`, `implemented_by`, `supersedes`, `superseded_by`, `blocks`, `blocked_by`, `references`, `referenced_by`.

## Non-goals

DevSpecs is **not**:

- A task manager, sprint board, or Linear/GitHub Issues replacement
- Cloud sync, accounts, or team workspaces
- Semantic search, embeddings, or automatic agent session mining
- CI/CD gates, approval workflows, or drift detection
- A docs hosting platform

**Todo model:** extracted checklist **observability** only — not owners, due dates, dependencies, assignment, or external sync.

## Troubleshooting

| Symptom | What to try |
|---------|-------------|
| Schema / version errors from the CLI | **`ds scan --rebuild`** (or delete `DEVSPECS_HOME/devspecs.db` per error text); migrations are bounded — use rebuild when the DB cannot be upgraded |
| No artifacts after scan | **`ds config show`** — confirm paths; add sources with **`ds config add-source`** or edit YAML |
| Unknown ID | **`ds list`**, **`ds find <term>`**, or **`ds resolve`** |
| Custom index location | Set **`DEVSPECS_HOME`** |
| Wrong repo in multi-repo filters | **`--repo`** takes the repo directory **basename** |

## Development

```bash
git clone https://github.com/devspecs-com/devspecs-cli.git
cd devspecs-cli
go test ./... -count=1
go run ./cmd/ds --help
```

Format check (matches CI):

```bash
gofmt -l .
go vet ./...
staticcheck ./...
```

### Maintaining format fixtures

Regression layouts under [`testdata/samples/`](testdata/samples/) (`bmad`, `specify`, `cursor`, `claude`, `codex`) support adapter tests.

[`testdata/samples/false-positives/`](testdata/samples/false-positives/) holds markdown (and similar) inputs that are useful regression targets when tightening todo extraction heuristics.

- **Spec Kit:** [spec-kit](https://github.com/github/spec-kit) — `specify init …`, then copy `specs/` (and `.specify/` if needed) into `testdata/samples/specify/`.
- **BMAD:** `npx bmad-method install`, run workflows, copy **`_bmad-output/planning-artifacts/`** into `testdata/samples/bmad/`.

After changes: **`go test ./internal/adapters/markdown/... -count=1`**.

## Releasing

Releases use [GoReleaser](https://goreleaser.com/) via GitHub Actions:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

[MIT License](LICENSE)
