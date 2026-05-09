# devspecs-cli

Local-first CLI for indexing, identifying, and referencing software planning/specification artifacts.

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

```bash
ds init        # Initialize DevSpecs in your repo
ds scan        # Scan for specs, plans, ADRs
ds list        # List indexed artifacts
ds show <id>   # Show artifact details
ds context <id> # Export agent-ready context
```

## Commands

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
  capture <path>      Capture a file as an artifact
  status <id> <s>     Update artifact status
  link <id> <target>  Add a link to an artifact
  version             Show version
```

### Global flags

All read commands support `--json` for machine-readable output.

### ds init

Creates `~/.devspecs/devspecs.db` (global index) and `.devspecs/config.yaml` (repo config).

```bash
ds init          # First-time setup
ds init --force  # Overwrite existing config
```

### ds scan

Discovers artifacts using configured adapters (OpenSpec, ADR, generic markdown).

```bash
ds scan              # Scan current directory
ds scan --path .     # Explicit path
ds scan --verbose    # Detailed output
ds scan --json       # JSON output
```

### ds todos

Lists extracted checklist items from all indexed artifacts.

```bash
ds todos             # All todos (defaults to open)
ds todos <id>        # Todos for a specific artifact
ds todos --open      # Only incomplete
ds todos --done      # Only completed
ds todos --json      # JSON output
```

## Supported artifact types

| Adapter | Detected paths | Kind |
|---------|---------------|------|
| OpenSpec | `openspec/changes/<id>/proposal.md` | `openspec_change` |
| ADR | `docs/adr/*.md`, `docs/adrs/*.md`, `adr/*.md`, `adrs/*.md`, `architecture/decisions/*.md` | `adr` |
| Markdown | `specs/**/*.md`, `docs/specs/**/*.md`, `plans/**/*.md`, `docs/plans/**/*.md` | `plan`, `spec`, `requirements`, `markdown_artifact` |

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
      - plans
```

## Schema

Global database: `~/.devspecs/devspecs.db`

Override location with `DEVSPECS_HOME` environment variable.

Tables: `repos`, `artifacts`, `artifact_revisions`, `sources`, `links`, `artifact_todos`.

### artifact_todos

Extracted checklist items are stored per-artifact per-revision. The source file is authoritative in v0 — todos are re-extracted on every scan when content changes.

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

### Todo JSON contract

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

### v0 todo model boundary

v0 DevSpecs todo model = extracted checklist observability.
Not task management. Not workflow state. Not a Linear replacement.

v0 does NOT implement: owners, due dates, dependencies, comments, assignment, GitHub/Linear sync, or approval workflow.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic failure |
| 2 | User input error (unknown ID, malformed config) |

## Statuses

Supported artifact statuses: `draft`, `proposed`, `approved`, `implementing`, `implemented`, `superseded`, `rejected`, `unknown`.

## Link types

Supported link types: `related`, `implements`, `implemented_by`, `supersedes`, `superseded_by`, `blocks`, `blocked_by`, `references`, `referenced_by`.

## Non-goals for v0

- Cloud sync, user accounts, team workspaces
- GitHub/Linear integration
- CI/CD gating, approval workflows
- Semantic search, embeddings
- Automatic agent session mining
- Incident tracing, drift detection

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub Actions:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

[MIT License](LICENSE)
