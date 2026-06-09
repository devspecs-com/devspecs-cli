# DevSpecs CLI

> Local-first CLI for AI coding work as addressable task slices.

DevSpecs helps developers and coding agents work from durable intent instead of
one-off chat context. It indexes the plans, specs, ADRs, PRDs, task notes, and
OpenSpec changes already in a repository, then helps start new work from
bounded, repo-grounded task slices with packed source/test context.

- Website: [devspecs.com](https://devspecs.com)
- Docs: [docs.devspecs.com](https://docs.devspecs.com)
- Public task transcript: [TASK_WORKFLOW_EXAMPLE.md](TASK_WORKFLOW_EXAMPLE.md)
- Public eval boundary: [EVALS.md](EVALS.md)
- Releases: [GitHub Releases](https://github.com/devspecs-com/devspecs-cli/releases)

## Why

AI-assisted development creates a lot of useful intent artifacts: plans,
checklists, ADRs, PRDs, RFCs, OpenSpec changes, implementation notes, and
follow-up summaries. They are valuable, but they often end up scattered across
repo folders, editor state, pull requests, and ad-hoc files.

DevSpecs gives those artifacts stable local identity and makes them useful to
the next human or agent session.

It has two launch jobs:

- **Greenfield:** start new AI coding work with packed repo context and one
  bounded next slice.
- **Brownfield:** recover existing intent artifacts and turn them into
  searchable, agent-usable context.

DevSpecs is not an autonomous agent, task manager, SaaS workspace, or hosted
memory layer. The CLI is the product; editors, agents, and MCP/slash surfaces
can wrap it later.

## Install

### macOS / Linux

```bash
brew install devspecs-com/tap/devspecs
```

or:

```bash
curl -fsSL https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.sh | sh
```

### Windows

```powershell
scoop bucket add devspecs https://github.com/devspecs-com/scoop-bucket
scoop install devspecs
```

or:

```powershell
irm https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.ps1 | iex
```

### Go

```bash
go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest
```

## Greenfield: bounded task slices

Use `ds task` when you are about to ask an agent to make a repo change and want
the first slice to be grounded before the agent grabs the whole roadmap.

```bash
ds task "Serve Swagger UI OAuth2 redirect from a custom docs redirect URL" \
  --profile code-change \
  --slice "Trace Swagger UI OAuth2 redirect flow and tests" \
  --slice "Wire custom docs redirect URL through FastAPI docs helpers" \
  --slice "Add regression coverage and docs examples"

ds task show A01
ds task prompt A01
ds task checkpoint A01 --stage validated --decision promote
ds task finish A01 --decision promote
ds task next <task-id>
```

What this gives you:

- an `A00` task index;
- `A01`, `A02`, ... slice plan/result artifacts;
- packed source, test, docs, and receipt context;
- a one-slice agent prompt;
- explicit decision gates: `promote`, `improve`, `rework`, `rollback`, `block`;
- lifecycle state from `start`, `checkpoint`, `finish`, `decide`, and `sync`.

See [TASK_WORKFLOW_EXAMPLE.md](TASK_WORKFLOW_EXAMPLE.md) for a public-safe
transcript generated from current CLI output.

## Brownfield: recover existing intent

Use scan/map/find when a repo already has plans, PRDs, RFCs, ADRs, specs,
runbooks, eval cards, or agent notes, but they are hard to find or hand to an
agent.

```bash
ds init
ds scan
ds map
ds find "oauth redirect"
ds find --pack "oauth redirect"
ds context <id>
```

`ds scan` builds the local index. `ds map` summarizes useful repo areas and
follow-up pack commands. Plain `ds find` locates indexed artifacts. Packed find
groups source, tests, docs, receipts, and exclusions into an agent-readable
context pack.

`ds adopt` is planned, not shipped. Current brownfield workflows scan and query
existing artifacts in place without mutating old files.

## What DevSpecs Indexes

DevSpecs currently indexes:

- OpenSpec changes;
- ADR directories such as `docs/adr`, `docs/adrs`, `adr`, and `adrs`;
- markdown plans/specs/PRDs/design docs under common paths such as `plans`,
  `docs/plans`, `docs/specs`, `.cursor/plans`, `docs/design`, and
  `docs/technical`;
- common agent and planning layouts, including Cursor, Spec Kit, BMAD output,
  Claude, and Codex samples used by tests;
- checklists, acceptance criteria, success criteria, and OKR-style criteria;
- task workspaces created by `ds task`.

Source files remain authoritative. DevSpecs stores derived index state locally.

## Core Commands

| Command | Use |
| --- | --- |
| `ds init` | Create local index state and repo config. |
| `ds scan` | Index configured intent-artifact paths. |
| `ds map` | Summarize repo areas and useful follow-up context commands. |
| `ds find <query>` | Search indexed artifacts. |
| `ds find --pack <query>` | Build agent-readable packed context. |
| `ds task <query>` | Create a bounded task workspace with slice artifacts. |
| `ds task show <target>` | Show exact context for one task target. |
| `ds task prompt <target>` | Emit an agent prompt bounded to one target. |
| `ds task checkpoint <target>` | Record files, tests, misses, noise, learnings, and next decision. |
| `ds task finish <target>` | Finish a target with a decision gate. |
| `ds task sync <task-id>` | Recapture edited task artifacts into the local index. |
| `ds resume` | Show in-progress, recently settled, and stale artifacts. |
| `ds todos` / `ds criteria` | List extracted checklist and criteria rows. |
| `ds context <id>` | Export one artifact as paste-ready agent context. |
| `ds config show` | Inspect effective repo discovery config. |

Most read commands support `--json`. Run `ds <command> --help` for the current
flag surface.

## Storage And Privacy

| Location | Role | Commit? |
| --- | --- | --- |
| `~/.devspecs/devspecs.db` | Local SQLite index and cache. | No. |
| `.devspecs/config.yaml` | Repo discovery configuration. | Usually yes. |
| `.devspecs/tasks/<task-id>/` | Current default generated task workspace. | Deliberately. |
| `devspecs/tasks/<task-id>/` | Visible task workspace path via `--dir devspecs/tasks`. | Yes, when durable. |

Telemetry is minimal and anonymous. It is used for install, init, scan, and
query flow health, and excludes repository names, file paths, git remotes,
artifact titles, document text, source code, and raw search queries.

Disable it with:

```bash
DEVSPECS_TELEMETRY=0
```

or:

```bash
DS_TELEMETRY=0
```

Use `DEVSPECS_TELEMETRY=debug` to print the would-send event to stderr.

## Public Eval Boundary

The public repo contains deterministic product tests and small synthetic
fixtures. It is the product claim surface, not a dump of exploratory research
material or unreduced evaluation runs. Public claims should stay tied to
reproducible public fixtures and documented behavior.

See [EVALS.md](EVALS.md) for the current boundary.

## Development

```bash
git clone https://github.com/devspecs-com/devspecs-cli.git
cd devspecs-cli
go test ./... -count=1
go run ./cmd/ds --help
```

Useful checks:

```bash
gofmt -l .
go vet ./...
staticcheck ./...
```

To enable the repo pre-commit hook:

```bash
make hooks
```

The hook runs `go vet`, `staticcheck`, `gofmt -l`, and by default
`go test -count=1 ./...`.

## Releasing

Releases use GoReleaser via GitHub Actions.

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

[MIT License](LICENSE)
