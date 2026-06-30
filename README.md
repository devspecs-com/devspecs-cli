# DevSpecs CLI

> Give agents the next slice, not the whole roadmap.

DevSpecs is a local-first CLI for AI coding workflows. It turns repo intent,
source, tests, docs, and recent work into bounded task slices with packed
context, checkpoints, and explicit decision gates.

No cloud required. No account. No LLM calls. No code upload. Your source files
stay authoritative.

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

## Try It In Five Minutes

Install:

```bash
brew install devspecs-com/tap/devspecs
```

Recover the local thread in the repo:

```bash
ds recent
```

Then open the LLM-oriented guide:

```bash
ds tldr
```

Create one bounded task:

```bash
ds task "fix OAuth redirect"
ds apply next
ds task checkpoint A01 --decision improve
ds apply next
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
| Recover the thread | `ds recent` | You are returning to a repo, checking active local work, or deciding what to ask next. |
| Bound an agent task | `ds task "goal"` | You know the work and want packed repo context plus a stop line. |
| Coordinate multi-repo work | `ds workspace init .` | You have an umbrella workspace with several child repos. Experimental. |
| Continue the next slice | `ds apply next` | A task already exists and the agent needs the current target only. |
| Record the receipt | `ds task checkpoint A01 --decision promote` | You need to capture what changed, what ran, and what comes next. |
| Map a repo | `ds map` | You are entering unfamiliar code and need system boundaries. |
| Inspect evidence | `ds find "topic"` | You want source, tests, docs, receipts, and exclusions in one context pack. |

## Why DevSpecs Exists

Issue trackers describe intended work. AI coding adds a new local work layer:
prompts, partial attempts, missed files, test evidence, course corrections,
and follow-up slices. Without structure, that layer disappears into chat logs
and editor state.

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

## Workspace Coordination

Workspace coordination is experimental and explicit. Use it only when one
umbrella directory coordinates work across several child repos. Normal
single-repo `ds task`, `ds task quick`, `ds apply`, and `ds task checkpoint`
remain the default path.

`ds ws` is a built-in shortcut for `ds workspace`; docs use the full command
when first introducing the workflow.

Current dogfood flow:

```powershell
ds workspace init . --json
ds workspace change create "Customer export across frontend/backend" --workspace . --repos backend,frontend --json
ds workspace slice create EAG-C001 --workspace . --repo backend --name "Backend API" --json
ds task show eag-c001-backend --repo ./enalytics-backend --json
ds apply eag-c001-backend --repo ./enalytics-backend --json
ds task checkpoint eag-c001-backend --repo ./enalytics-backend --target A01 --stage validated --decision promote --json
ds workspace trace EAG-C001 --workspace . --json
```

Workspace files are written under the umbrella `devspecs/` directory. Repo-local
task files are written under the selected child repo. The `--repo` flag is the
explicit boundary between the current shell directory and the target repo for
task/apply/checkpoint work.

`ds workspace trace` is for known workspace change or repo task IDs. Use
`ds find` when you need to discover relevant source, tests, docs, or prior task
receipts.

Migration guarantee: new docs, help text, and examples use the
`ds workspace ...` namespace. Top-level `ds change`, `ds slice`, and `ds trace`
remain hidden compatibility aliases for older local scripts. Regression tests
cover three gates: root help hides the aliases, alias help points to the
workspace form, and the aliases still dispatch the same workspace operations.

## Command Roles

| Need | Command | Meaning |
| --- | --- | --- |
| Discover evidence | `ds find "topic"` | Pack likely source, tests, docs, receipts, and exclusions for a focused question. |
| Check task progress | `ds task status/next/show` | Read lifecycle state from task manifests, checkpoints, stages, and decisions. |
| Follow workspace links | `ds workspace trace <id>` | Trace a known workspace change or repo task to linked repo-local slices. |

`ds workspace trace` reports both lifecycle `status` and index-capture
`index_status`. Keep them separate: `index_missing` means an artifact is not
currently captured in the local index; it is not the same as `missing_result`.

## Trust Layer

Use these commands when scope is unclear or you want to verify what the agent is
about to use:

```bash
ds recent
ds find "oauth redirect"
ds map
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
| `ds recent [topic]` | Start here to recover the local thread, recently active topics, and follow-up context commands. |
| `ds init` | Create local index state, repo config, and optional agent adapter files. |
| `ds tldr [workflow]` | Show LLM-oriented quickstarts for setup, hotfixes, epics, incidents, brownfield recovery, handoff, and deep dives. |
| `ds task <query>` | Create a bounded task workspace with slice artifacts. |
| `ds task quick <query>` | Create a compact one-off task workspace. |
| `ds task status/next/show` | Inspect task lifecycle state and choose the next target. |
| `ds apply <next\|task-id\|target>` | Emit the next bounded one-slice agent prompt without mutating task state. |
| `ds task checkpoint <target>` | Record files, tests, misses, noise, learnings, decision evidence, and next iteration. |
| `ds task refresh <task-id>` | Recapture edited task artifacts into the local index without rewriting task docs. |
| `ds workspace init/show/change/slice/trace` | Coordinate experimental workspace-level changes, repo-local task slices, and known change/task traces. |
| `ds map` | Show architecture/system boundaries with evidence and follow-up commands. |
| `ds find <query>` | Build agent-readable packed context. |
| `ds context <id>` | Export one artifact as paste-ready agent context. |
| `ds scan` | Manually refresh or rebuild configured intent-artifact paths. |
| `ds config show` | Inspect effective repo discovery config. |

Most read commands support `--json`. Run `ds <command> --help` for the current
flag surface. Prefer the `ds workspace ...` form for workspace coordination;
top-level workspace aliases are hidden compatibility shims.

## Storage And Privacy

| Location | Role | Commit? |
| --- | --- | --- |
| `~/.devspecs/devspecs.db` | Local SQLite index and cache. | No. |
| `.devspecs/config.yaml` | Repo discovery configuration. | Usually yes. |
| `devspecs/tasks/<task-id>/` | Default generated task workspace. | Yes, when durable. |
| `.devspecs/tasks/<task-id>/` | Legacy or explicitly local task workspace. | No, unless you chose it deliberately. |
| `devspecs/workspace.yaml` | Experimental workspace manifest for umbrella repos. | Yes, when used by the team. |
| `devspecs/changes/<change-id>-*.md` | Experimental workspace-level change records. | Yes, when used by the team. |

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
in place through `ds recent`, `ds find`, `ds map`, and `ds scan`. `ds adopt` is
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
