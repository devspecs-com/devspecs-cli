# DevSpecs CLI

> Stop losing the thread.

DevSpecs keeps the durable parts of AI coding work attached to your repo, so
humans and agents can continue without reconstructing the thread from chat.

Git shows what changed. DevSpecs shows what matters next: recent work, packed
repo evidence, task state, decision gates, checkpoints, and the next bounded
handoff.

Use it as a lightweight task/spec workflow, or as a local codebase navigation
layer for the plans, ADRs, PRDs, RFCs, docs, source, tests, and git history you
already have.

Local-first. No cloud sync. No account. No LLM calls. No code upload. Your
source files stay authoritative.

<p>
  <a href="https://devspecs.com">
    <img src="https://devspecs.com/demo/fastapi-task-flow-v1-1.gif" alt="DevSpecs FastAPI task flow demo" width="900">
  </a>
</p>

## Links

| | |
| --- | --- |
| Website | [devspecs.com](https://devspecs.com) |
| Docs | [docs.devspecs.com](https://docs.devspecs.com) |
| Task transcript | [TASK_WORKFLOW_EXAMPLE.md](TASK_WORKFLOW_EXAMPLE.md) |
| Releases | [GitHub Releases](https://github.com/devspecs-com/devspecs-cli/releases) |
| Changelog | [CHANGELOG.md](CHANGELOG.md) |
| X | [@brennan_maker](https://x.com/brennan_maker) |
| Reddit | [u/bnunamak](https://www.reddit.com/user/bnunamak/) |
| LinkedIn | [Brennan Nunamaker](https://www.linkedin.com/in/brennan-nunamaker-30657a70) |

## Install, Then Try It

Install:

```bash
brew install devspecs-com/tap/devspecs
```

Start with the LLM-oriented guide:

```bash
ds tldr
```

Recover the local thread:

```bash
ds recent
```

Create one bounded task in your repo:

```bash
ds task "fix OAuth redirect"
ds apply next
ds task checkpoint A01 --decision improve
ds apply next
```

Or try it in a disposable FastAPI checkout:

```bash
git clone https://github.com/fastapi/fastapi
cd fastapi
ds init
ds recent
ds task "trace Swagger OAuth redirect behavior"
```

Or let DevSpecs write thin adapter files for Codex, Cursor, Claude, and
Windsurf:

```bash
ds init
# then, when your tool supports it:
/ds-task "fix OAuth redirect"
```

## What It Helps With

| Job | Command | Use When |
| --- | --- | --- |
| Recover the thread | `ds recent` | You came back cold and need the current local work thread. |
| Ground the change | `ds map` / `ds find "topic"` | Git and rg found code, but you still need intent, boundaries, and exclusions. |
| Create a bounded handoff | `ds task "goal"` | You know the work and want packed repo context plus a stop line. |
| Continue one slice | `ds apply next` | A task already exists and the agent needs the current target only. |
| Record the receipt | `ds task checkpoint A01 --decision promote` | You need to capture what changed, what ran, what missed, and what comes next. |
| Inspect exact context | `ds context <artifact-id>` | You want one indexed artifact as paste-ready agent context. |

## Why DevSpecs Exists

Issue trackers describe intended work. Git records what changed. AI coding adds
a new local work layer between them: prompts, partial attempts, missed files,
test evidence, course corrections, and follow-up slices.

Without structure, that layer disappears into chat logs and editor state. The
next human or agent has to infer why the branch exists, what passed, what was
superseded, and where to continue.

DevSpecs gives that layer local shape:

- task slices that tell the agent where to stop;
- packed source, test, docs, and intent context before implementation starts;
- explicit gates: `promote`, `improve`, `rework`, `rollback`, and `block`;
- iteration slices such as `A01-1` and `A01-2` when the first attempt teaches
  you something;
- checkpoint and result artifacts that survive compaction, handoff, and the
  next agent session.

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

After installing or upgrading, restart your shell or IDE terminal if `ds` is
not found.

```bash
ds version
ds update
```

`ds update` is guidance-only. It shows the active binary, likely install source,
latest release status, and the update command to run.

## Task-First Workflow

For multi-slice features, migrations, architecture work, or anything likely to
drift, create explicit slices:

```bash
ds task "Serve Swagger UI OAuth2 redirect from a custom docs redirect URL" \
  --profile code-change \
  --slice "Trace Swagger UI OAuth2 redirect flow and tests" \
  --slice "Wire custom docs redirect URL through FastAPI docs helpers" \
  --slice "Add regression coverage and docs examples"

ds task show A01
ds apply next
ds task checkpoint A01 --decision promote --next-target A02
ds apply next
```

What you get:

- `A00` task index;
- `A01`, `A02`, ... slice plan/result artifacts;
- packed source, test, docs, and receipt context;
- a one-slice agent prompt;
- lifecycle state from `start`, `checkpoint`, `finish`, `decide`, and
  `refresh`;
- a durable record of what changed, what ran, what missed, and what should
  happen next.

For a smaller one-off:

```bash
ds task quick "Fix discount rounding in invoice totals"
```

Use full `ds task` when you want durable slices and handoff receipts. Use
`ds task quick` when the ceremony would outweigh the change.

## Trust Layer

Use these commands when scope is unclear or you want to verify what the agent is
about to use:

```bash
ds map
ds recent
ds find "oauth redirect"
ds context <artifact-id>
```

The trust layer is diagnostic. It should route you to current owner intent,
source, tests, docs, recent changes, and exclusions. It does not replace reading
the owner decision doc when one exists.

`ds find` returns an agent-readable pack by default. Use `ds find --plain` for
the older flat ranked result list.

## What DevSpecs Indexes

DevSpecs indexes the substrate your repo already has:

- plans, specs, PRDs, RFCs, ADRs, runbooks, and decision memos;
- OpenSpec changes;
- task workspaces created by `ds task`;
- source, tests, docs, config, and recent git activity used for maps and
  evidence packs;
- checklists, acceptance criteria, success criteria, and OKR-style criteria;
- common agent/planning layouts such as Cursor, Codex, Claude, Spec Kit, and
  BMAD samples used by tests.

Index state lives in local SQLite and can be rebuilt.

## Command Map

| Command | Use |
| --- | --- |
| `ds init` | Create local index state, repo config, and optional agent adapter files. |
| `ds tldr [workflow]` | Show LLM-oriented quickstarts for setup, hotfixes, epics, incidents, brownfield recovery, handoff, and deep dives. |
| `ds task <query>` | Create a bounded task workspace with slice artifacts. |
| `ds task quick <query>` | Create a compact one-off task workspace. |
| `ds apply <next\|task-id\|target>` | Emit the next bounded one-slice agent prompt without mutating task state. |
| `ds task checkpoint <target>` | Record files, tests, misses, noise, learnings, decision evidence, and next iteration. |
| `ds task refresh <task-id>` | Recapture edited task artifacts into the local index without rewriting task docs. |
| `ds map` | Show architecture/system boundaries with evidence and follow-up commands. |
| `ds recent` | Show recently active local git topics and follow-up context commands. |
| `ds find <query>` | Build agent-readable packed context. |
| `ds context <id>` | Export one artifact as paste-ready agent context. |
| `ds scan` | Manually refresh or rebuild configured intent-artifact paths. |
| `ds config show` | Inspect effective repo discovery config. |

Most read commands support `--json`. Run `ds <command> --help` for the current
flag surface.

## Storage And Privacy

| Location | Role | Commit? |
| --- | --- | --- |
| `~/.devspecs/devspecs.db` | Local SQLite index and cache. | No. |
| `.devspecs/config.yaml` | Repo discovery configuration. | Usually yes. |
| `devspecs/tasks/<task-id>/` | Default generated task workspace. | Yes, when durable. |
| `.devspecs/tasks/<task-id>/` | Legacy or explicitly local task workspace. | No, unless you chose it deliberately. |

Commit task artifacts when they explain durable work, should be reviewed with a
change, or are useful to the next person or agent. If a task is scratch-only,
ignore `devspecs/tasks/<task-id>/` yourself or use:

```bash
ds task "scratch goal" --dir .devspecs/tasks
```

Telemetry is minimal and anonymous. It excludes repository names, file paths,
git remotes, artifact titles, document text, source code, and raw search
queries.

Disable it with:

```bash
DEVSPECS_TELEMETRY=0
```

or:

```bash
DS_TELEMETRY=0
```

Use `DEVSPECS_TELEMETRY=debug` to print the would-send event to stderr.

## FAQ

### Does DevSpecs call an LLM?

No. DevSpecs is a local CLI. It creates context and prompts for agents, but it
does not call a model itself.

### Does DevSpecs upload code or plans?

No. The index is local SQLite. Source files remain authoritative. Optional
telemetry is anonymous and excludes repo names, file paths, document text,
source code, and raw queries.

### Is this a spec framework like OpenSpec?

Partly, but DevSpecs is broader. It can create lightweight task specs with
packed source, tests, intent, decision gates, iteration slices, and checkpoints.
It also works as a local codebase navigation layer by indexing existing plans,
ADRs, PRDs, RFCs, docs, source, tests, git history, and task state without
requiring a new spec process first.

### Do I need MCP or slash commands?

No. The CLI is the product. `ds init` can generate thin adapter files for agent
tools, but those wrappers route back through `ds task` and `ds apply`.

### Why not just use epics, stories, and tasks?

Traditional issue trackers describe planned work. Agent work creates local
attempts, misses, evidence, and iteration slices between ticket updates.
DevSpecs manages that local AI work layer without replacing the tracker.

### Should I commit `devspecs/tasks`?

Commit durable task artifacts when they help the team understand or review the
work. Use `.devspecs/tasks` or a gitignored path for scratch-only local plans.

### Is `ds adopt` available?

Not yet. Current brownfield workflows already index existing intent artifacts
in place through `ds map`, `ds find`, `ds recent`, and `ds scan`. `ds adopt` is
planned for creating thin wrapper artifacts without mutating old PRDs, RFCs,
ADRs, or plans.

### Is `ds find` a replacement for reading plans?

No. `ds find` is a routing and evidence layer. If it surfaces a current owner
decision memo, north-star doc, or `Status: next` plan, read that artifact before
asking the agent to change code.

## Public Eval Boundary

The public repo contains deterministic product tests and small synthetic
fixtures. It is the product claim surface, not a dump of exploratory research
material or unreduced evaluation runs.

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
git tag v1.1.0
git push origin v1.1.0
```

## License

[MIT License](LICENSE)
