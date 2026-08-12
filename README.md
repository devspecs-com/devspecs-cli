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
    <img src="https://devspecs.com/demo/fastapi-recent-gate-v1-1.gif" alt="DevSpecs FastAPI recent work demo" width="900">
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

Recover the local thread:

```bash
ds recent
```

When you want a compact agent cheat sheet:

```bash
ds tldr
```

Create one bounded task in your repo:

```bash
ds task "fix OAuth redirect"
ds apply
ds task checkpoint A01 --decision improve
ds apply
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
| Coordinate multi-repo work | `ds workspace init .` | You have an umbrella workspace with several child repos. Experimental. |
| Continue one slice | `ds apply` | A task already exists and the agent needs the current target only. |
| Coordinate parallel lanes | `ds thread task:<task-id>` | One task has several independently runnable slices or an explicit join. Experimental. |
| Record the receipt | `ds task checkpoint A01 --decision promote` | You need to capture what changed, what ran, what missed, and what comes next. |
| Preserve a durable decision | `ds compose adr "title"` | A track settled a consequential technical choice that should outlive task history. Experimental. |
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
- follow-up slices such as `A01-1` and `A01-2` when the first attempt teaches
  you something, created with `ds task slice add <task-id> "<title>" --after A01`;
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
ds apply
ds task checkpoint A01 --decision promote --next-target A02
ds apply
```

What you get:

- `A00` task index;
- `A01`, `A02`, ... slice plan/result artifacts;
- packed source, test, docs, and receipt context;
- a one-slice agent prompt;
- lifecycle state from `checkpoint`, `status`, and `refresh`;
- a durable record of what changed, what ran, what missed, and what should
  happen next.

After the implementation slices finish, new full task tracks expose one closeout
target. Record whether the work needs a durable repo document; this happens once
per track, not once per slice. Compact `--quick` tasks skip this review:

```bash
ds compose adr "Keep one writer lease per index" --from-task index-reliability --target A
ds task checkpoint index-reliability --target A00 --stage completed --decision complete \
  --durable-record recorded --durable-artifact docs/adr/0007-keep-one-writer-lease-per-index.md
```

Use `--durable-record none` for local, obvious, reversible implementation
details. Use `deferred --next-target <target>` only when the durable record has
a named owner or follow-up.

For a smaller one-off:

```bash
ds task "Fix discount rounding in invoice totals" --quick
```

Use full `ds task` when you want durable slices and handoff receipts. Add
`--quick` when the ceremony would outweigh the change.

## Named Execution Threads

Named threads are experimental scheduling lanes over existing task slices.
Ordinary tasks own their threads in the repository; no workspace is required:

```bash
ds thread set task:checkout-redesign agent-a A01 A02
ds thread set task:checkout-redesign human-a A03
ds thread set task:checkout-redesign both-a A04 --after agent-a --after human-a
ds thread task:checkout-redesign
ds apply task:checkout-redesign --thread agent-a
```

Argument-free `ds apply` still works for a legacy linear task or when exactly
one named lane is runnable. When several lanes are runnable, DevSpecs does not
pick the first one: `ds thread` shows every lane and prints exact `ds apply
--thread` commands.

A task escalates to workspace-change ownership only when its manifest explicitly
links that change. Merely living inside an umbrella directory does not change
ownership. Cross-repo graphs use the same root `ds thread` and `ds apply`
commands with `change:<id>` and `--workspace`; there is no `ds workspace thread`.
After every cross-repo lane completes, the workspace change is terminal. Run
`ds apply <linked-task> --repo <child-repo>` for each repo that still needs its
one-time `A00`-style durable-record closeout.

## Workspace Coordination

Workspace coordination is experimental and explicit. Use it only when one
umbrella directory coordinates work across several child repos. Normal
single-repo `ds task`, `ds task --quick`, `ds apply`, and `ds task checkpoint`
remain the default path.

`ds ws` is a built-in shortcut for `ds workspace`; docs use the full command
when first introducing the workflow.

Example:

```powershell
ds workspace init . --json
ds workspace change create "Customer export across frontend/backend" --workspace . --repos backend,frontend --json
ds workspace slice create EAG-C001 --workspace . --repo backend --name "Backend API" --json
ds task show eag-c001-backend --repo ./enalytics-backend --json
ds apply eag-c001-backend --repo ./enalytics-backend --json
ds task checkpoint eag-c001-backend --repo ./enalytics-backend --target A01 --stage validated --decision promote --json
ds compose adr "Define the export ownership boundary" --repo ./enalytics-backend --from-task eag-c001-backend --target A00 --json
ds workspace trace EAG-C001 --workspace . --json
```

Workspace files are written under the umbrella `devspecs/` directory. Repo-local
task files are written under the selected child repo. The `--repo` flag is the
explicit boundary between the current shell directory and the target repo for
task, apply, checkpoint, and compose work. There is no `ds workspace compose`:
durable documents belong to one versioned repository. Use the umbrella
repository only when it is itself the canonical owner of a cross-repo document.

`ds workspace trace` is for known workspace change or repo task IDs. Use
`ds find` when you need to discover relevant source, tests, docs, or prior task
receipts.

## Command Roles

| Need | Command | Meaning |
| --- | --- | --- |
| Discover evidence | `ds find "topic"` | Pack likely source, tests, docs, receipts, and exclusions for a focused question. |
| Check task progress | `ds task status/show` + `ds apply` | Read lifecycle state, inspect one target, then emit the bounded prompt. |
| Follow workspace links | `ds workspace trace <id>` | Trace a known workspace change or repo task to linked repo-local slices. |

## Durable Documents

`ds compose` creates a Markdown draft in the repository, captures it into the
index, and keeps it separate from prunable task receipts:

```bash
ds compose adr "Use an append-only event log"
ds compose rfc "Coordinate concurrent index writers"
ds compose prd "Reliable cold activation"
```

Use an ADR after a meaningful technical choice is settled, an RFC while a
consequential proposal still needs review, and a PRD for a product problem,
users, outcomes, and requirements spanning one or more tracks. Skip small,
obvious, reversible choices.

The Markdown document is the source of truth. Its index row is derived state:
`ds prune` never deletes repository files, and `ds scan --rebuild` rediscovers
documents in configured or conventional ADR/RFC/PRD paths. From an umbrella
workspace, pass `--repo <child-repo>` to keep ownership explicit.
When `--output` selects an unconventional directory, add that directory to
`.devspecs/config.yaml` for deterministic rebuild discovery.

ADR format defaults to `auto`: DevSpecs reuses a recognized repo convention and
fails on ambiguous precedent. With no precedent it uses Nygard. Choose explicitly
with `--format nygard|madr|y-statement|outcome-first|iso-42010`; MADR also accepts
`--variant full|minimal`. Run `ds compose adr --help` for the compact format
selection guide.

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

DevSpecs indexes the context your repo already has:

- plans, specs, PRDs, RFCs, ADRs, runbooks, and decision memos;
- OpenSpec changes;
- task workspaces created by `ds task`;
- source, tests, docs, config, and recent git activity used for maps and
  evidence packs;
- checklists, acceptance criteria, success criteria, and OKR-style criteria;
- common agent/planning layouts such as Cursor, Codex, Claude, Spec Kit, and
  BMAD samples used by tests.

Index state lives in local SQLite and can be rebuilt. Git worktrees that share
the same canonical remote and root commit reuse one logical repository index
instead of indexing every worktree as a new repository.

## Command Map

| Command | Use |
| --- | --- |
| `ds recent [topic]` | Start here to recover the local thread, recently active topics, and follow-up context commands. |
| `ds init` | Create local index state, repo config, and optional agent adapter files. |
| `ds tldr [workflow]` | Show LLM-oriented quickstarts for setup, hotfixes, epics, incidents, brownfield recovery, handoff, and deep dives. |
| `ds task <query>` | Create a bounded task workspace with slice artifacts. |
| `ds task <query> --quick` | Create a compact one-off task workspace. |
| `ds task status/show` | Inspect task lifecycle state and target context. |
| `ds thread [task:<task-id>\|change:<change-id>]` | Inspect named lanes, joins, runnable targets, and exact apply commands. Experimental. |
| `ds apply [task-id\|change-id\|target] [--thread <key>]` | Emit one bounded prompt without mutating task state; omit lane selection only when the next target is unambiguous. |
| `ds task checkpoint <task-id\|target>` | Record files, tests, misses, noise, learnings, decision evidence, and next iteration. |
| `ds compose adr\|rfc\|prd "<title>"` | Create and index a repo-owned durable draft using established repository conventions. Experimental. |
| `ds task slice add <task-id> "<title>" --after A01 --reason improve` | Add an A01-1-style follow-up slice after an improve/rework gate. |
| `ds task refresh <task-id>` | Recapture edited task artifacts into the local index without rewriting task docs. |
| `ds workspace init/show/change/slice/trace` | Coordinate experimental workspace-level changes, repo-local task slices, and known change/task traces. |
| `ds map` | Show architecture/system boundaries with evidence and follow-up commands. |
| `ds find <query>` | Build agent-readable packed context. |
| `ds context <id>` | Export one artifact as paste-ready agent context. |
| `ds scan` | Manually refresh or rebuild configured intent-artifact paths. |
| `ds prune [--dry-run] [--vacuum]` | Remove stale repository data and redundant capture revisions; compact the database explicitly with `--vacuum`. |
| `ds config show` | Inspect effective repo discovery config. |

Most read commands support `--json`. Run `ds <command> --help` for the current
flags. Use the `ds workspace ...` form for workspace coordination.

## Storage And Privacy

| Location | Role | Commit? |
| --- | --- | --- |
| `~/.devspecs/devspecs.db` | Local SQLite index and cache. | No. |
| `.devspecs/config.yaml` | Repo discovery configuration. | Usually yes. |
| `devspecs/tasks/<task-id>/` | Default generated task workspace. | Yes, when durable. |
| `.devspecs/tasks/<task-id>/` | Legacy or explicitly local task workspace. | No, unless you chose it deliberately. |
| `devspecs/tasks/<task-id>/threads.yaml` | Repo-owned named thread definition. | Yes. |
| `devspecs/tasks/<task-id>/checkpoints/*.json` | Immutable task lifecycle events; current thread state is derived from event heads. | Yes, when the task is durable. |
| Repository ADR/RFC/PRD paths, such as `docs/adr/`, `docs/rfcs/`, and `docs/prd/` | Canonical durable documents created or reused by `ds compose`. | Yes. |
| `devspecs/workspace.yaml` | Experimental workspace manifest for umbrella repos. | Yes, when used by the team. |
| `devspecs/changes/<change-id>-*.md` | Experimental workspace-level change records. | Yes, when used by the team. |
| `devspecs/changes/<change-id>.threads.yaml` | Cross-repo thread definition for one explicitly linked workspace change. | Yes, when used by the team. |

Checkpoint JSON and thread-definition YAML are durable authority. `task.json`
keeps the task definition plus compatibility lifecycle fields, while SQLite is
a rebuildable local projection for fast reads. DevSpecs does not currently
write a JSONL thread stream; a future JSONL export may be generated from the
immutable events, but would not become authoritative.

The product boundaries are deliberate: tasks hold bounded execution evidence,
threads schedule existing targets, compose creates durable ADR/RFC/PRD files,
workspaces optionally coordinate explicitly linked repositories, and prune
maintains derived index state without deleting those repository artifacts.

The global index records every physical root observed for a logical Git
repository. This lets temporary agent worktrees reuse the repository's index.
When worktrees are deleted, clean their stale aliases and data with:

```bash
ds prune --dry-run
ds prune
ds prune --vacuum
```

`ds prune` only removes a logical repository when none of its recorded roots
still exist. It also collapses consecutive capture revisions with identical
content while preserving the current revision and distinct content transitions.
It never removes source files, composed documents, task artifacts, or workspace
records from disk.
Normal pruning makes freed SQLite pages reusable. `--vacuum` also rewrites the
database to return unused space to the filesystem, which can take time and
require temporary free disk space on a large index. Long human-mode maintenance
operations report bounded progress on stderr; JSON output remains clean.

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
attempts, misses, evidence, and follow-up slices between ticket updates.
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

Before tagging a release candidate, run the local gate:

```bash
gofmt -l .
go vet ./...
staticcheck ./...
go test -count=1 ./...
```

```bash
git tag v1.2.0
git push origin v1.2.0
```

## License

[MIT License](LICENSE)
