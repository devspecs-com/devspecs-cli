# DevSpecs CLI

[![CI](https://github.com/devspecs-com/devspecs-cli/actions/workflows/go.yml/badge.svg)](https://github.com/devspecs-com/devspecs-cli/actions/workflows/go.yml)
[![Release](https://img.shields.io/github/v/release/devspecs-com/devspecs-cli)](https://github.com/devspecs-com/devspecs-cli/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

> Stop losing the thread.

AI coding creates a work layer that Git and issue trackers do not preserve:
partial attempts, packed context, decisions, test evidence, and the next safe
slice.

DevSpecs keeps that durable thread in versionable repository artifacts and a
rebuildable local index. When you return to an agent's work, you can see why the
change exists, what passed, what was superseded, and where to continue.

Local-first. No account. No cloud sync. No LLM calls. No code upload.

<p>
  <a href="https://devspecs.com">
    <img src="https://devspecs.com/demo/fastapi-recent-gate-v1-1.gif" alt="DevSpecs recovering recent FastAPI work" width="900">
  </a>
</p>

## Try It In Five Minutes

Install on macOS or Linux:

```bash
brew install devspecs-com/tap/devspecs
```

Install on Windows:

```powershell
scoop bucket add devspecs https://github.com/devspecs-com/scoop-bucket
scoop install devspecs
```

Try it in a disposable FastAPI checkout:

```bash
git clone https://github.com/fastapi/fastapi
cd fastapi
ds init
ds recent
```

The first run orients you in the existing repository:

```text
Recently active topics
Repo: fastapi

1. App Frontend Automatic Cookie
   Evidence: 1 commit, 3 files, source + tests + docs
   Key files:
   - fastapi/routing.py
   - tests/test_frontend.py
   - docs/en/docs/tutorial/frontend.md
   Try: ds find "app frontend automatic cookie"
```

This is a selected excerpt from a current-source run against FastAPI commit
[`7cb06f3`](https://github.com/fastapi/fastapi/tree/7cb06f360dd44efac059848df1a9beee7643b018).
Only later topics were omitted and the temporary checkout name was normalized.

Hand DevSpecs to your coding agent with:

```bash
ds tldr
```

It prints the current workflow rules and exact commands for common jobs.
`ds init` can also write thin adapter files for Codex, Cursor, Claude, and
Windsurf. The CLI remains the source of truth.

## Three Core Workflows

### Come Back After An Agent

```bash
ds recent
```

Recover recently active topics and relevant changes, then follow exact commands
into task or evidence state. Use a topic when you know roughly what you are
returning to:

```bash
ds recent "OAuth redirect"
```

### Ground An Unfamiliar Change

```bash
ds map
ds find "OAuth redirect"
```

Connect plans and decisions to likely source, tests, recent commits, and
excluded noise before an agent edits.

### Carry One Bounded Slice Forward

```bash
ds task "protect custom OAuth redirect behavior" \
  --id oauth-redirect \
  --slice "trace redirect behavior and tests" \
  --slice "add regression coverage"

ds apply oauth-redirect
ds task checkpoint oauth-redirect --target A01 --decision improve
```

`ds task` packs repo evidence into addressable slice plans. `ds apply` emits
one bounded agent prompt. `checkpoint` records what changed, what ran, what was
missed, and which decision gate the evidence earned.

When an attempt teaches you something, keep the learning attached to that
slice:

```bash
ds task slice add oauth-redirect \
  "verify the newly discovered route owner" \
  --after A01 \
  --reason improve
```

That creates an `A01-1` style follow-up instead of pretending the first plan
was complete. The normal iteration gates are `promote`, `improve`, `rework`,
`rollback`, and `block`.

For a small one-off where a full track would be overhead:

```bash
ds task "fix discount rounding" --quick
```

See the [task workflow guide](https://docs.devspecs.com/greenfield/task-flow)
and the [public task transcript](TASK_WORKFLOW_EXAMPLE.md).

## Built For Brownfield Evidence

Repository context is not one undifferentiated pile of text. A current
decision, implementation file, regression test, stale workaround, and prior
checkpoint carry different authority.

DevSpecs assembles context by role and lifecycle instead of returning an
unexplained list of text matches. A pack can distinguish:

- current plans and decisions;
- implementation source;
- behavior tests;
- open work and prior checkpoint evidence;
- superseded or likely distracting context.

The pack keeps reasons and exclusions visible so an agent can inspect why a
file was admitted. No model or embedding service is required. Existing plans,
ADRs, PRDs, RFCs, source, tests, docs, and Git history remain authoritative.

Read more about [recovering existing intent](https://docs.devspecs.com/brownfield/recover-intent).

## Inspect The Evidence

| Evidence | What it shows |
| --- | --- |
| [Review agent work after returning](https://devspecs.com/examples/review-agent-work-after-returning) | A pinned FastAPI workflow combining physical changes, task state, and a human gate. |
| [Start a bounded code change](https://devspecs.com/examples/start-a-bounded-code-change) | Explicit slices, packed evidence, a one-target handoff, and a checkpoint. |
| [Public task transcript](TASK_WORKFLOW_EXAMPLE.md) | Real CLI commands, generated artifacts, and normalized output. |
| [CI](https://github.com/devspecs-com/devspecs-cli/actions/workflows/go.yml) | Current automated verification. |
| [Open issues](https://github.com/devspecs-com/devspecs-cli/issues) | Known limitations, bug reports, and planned repairs. |

DevSpecs is used repeatedly in private professional repositories by its author
and independent engineers. Those repositories stay private; public examples
use pinned open-source commits and reviewed CLI output.

## Where It Still Needs Judgment

- If a repository has no current intent artifact, DevSpecs can recover evidence
  but cannot invent authoritative intent.
- `ds map` and `ds find` provide grounded leads, not automatic ownership proof.
- A low-completeness pack is a reason to verify or stop, not permission to edit.
- The CLI does not infer a thoughtful multi-slice track from a one-line goal.
  Pass `--slice` arguments or let your agent formulate them first.
- Full task tracks add unnecessary ceremony to tiny fixes. Use `--quick` when
  the receipt would cost more than the change.
- Workspace coordination, named execution threads, and composed documents are
  experimental surfaces.

The trust layer should route you to owner intent and concrete evidence. It does
not replace reading the owner decision document when one exists.

## Core Commands

| Need | Command |
| --- | --- |
| Give an agent the workflow rules | `ds tldr` |
| Initialize a repo and optional adapters | `ds init` |
| Recover recently active work | `ds recent [topic]` |
| See system boundaries | `ds map [scope]` |
| Pack evidence for a focused question | `ds find "topic"` |
| Create a bounded task | `ds task "goal" [--slice "..."]` |
| Emit the next bounded prompt | `ds apply [task-id]` |
| Inspect task state | `ds task status <task-id>` |
| Record evidence and a decision | `ds task checkpoint <task-id> --target A01` |
| Add an iteration slice | `ds task slice add <task-id> "title" --after A01` |
| Refresh the local index explicitly | `ds scan` |

Most read commands support `--json`. Run `ds <command> --help` for the current
flags. The [command reference](https://docs.devspecs.com/commands) covers the
complete surface.

## Advanced Workflows

DevSpecs also supports experimental named execution threads, repo-owned
ADR/RFC/PRD drafts, and umbrella workspaces that coordinate repo-local tasks.
These remain outside the shortest adoption path:

```bash
ds thread task:<task-id>
ds compose adr "title"
ds workspace init .
```

Use `ds workspace ...` when one umbrella directory coordinates several child
repositories. Ordinary tasks remain repo-owned. `ds ws` is the built-in
shortcut for `ds workspace`.

## Installation Alternatives

<details>
<summary>macOS and Linux shell installer</summary>

```bash
curl -fsSL https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.sh | sh
```

</details>

<details>
<summary>Windows PowerShell installer</summary>

```powershell
irm https://raw.githubusercontent.com/devspecs-com/devspecs-cli/main/install.ps1 | iex
```

</details>

<details>
<summary>Go install</summary>

```bash
go install github.com/devspecs-com/devspecs-cli/cmd/ds@latest
```

</details>

After installing or upgrading, restart your shell or IDE terminal if `ds` is
not found. `ds update` reports the current release and the appropriate package
manager update command.

## Storage And Privacy

- `~/.devspecs/devspecs.db` is a local, rebuildable SQLite index.
- `.devspecs/config.yaml` stores repository discovery configuration.
- `devspecs/tasks/<task-id>/` is the default visible task workspace.
- Existing repository files remain the source of truth.
- `ds prune` removes derived index state, never repository files.

Commit task artifacts when they explain durable work or should be reviewed with
the change. Use `--dir .devspecs/tasks` for deliberately local scratch work.

Telemetry is minimal and anonymous. It excludes repository names, paths,
remotes, titles, document text, source code, and raw queries. Disable it with:

```bash
DEVSPECS_TELEMETRY=0
```

See [storage](https://docs.devspecs.com/storage) and
[privacy](https://docs.devspecs.com/privacy) for the complete boundaries.

## FAQ

### Does DevSpecs call an LLM?

No. It prepares context and prompts for the coding agents you already use.

### Is this a spec framework like OpenSpec?

Partly, but DevSpecs is broader. It can create lightweight task specs with
packed repo evidence, decision gates, iteration slices, and checkpoints. It
also indexes the intent artifacts and codebase you already have without
requiring a new spec process first.

### Does this replace Jira, Linear, or GitHub Issues?

No. Those systems describe planned work. DevSpecs preserves the local attempts,
evidence, decisions, and follow-up slices produced between ticket updates.

### Do I need MCP or slash commands?

No. The CLI is the product. Generated adapters are thin entry points into the
same commands.

### Should I commit `devspecs/tasks`?

Commit durable task artifacts when they help a team review or continue the
work. Use a local or ignored path for scratch-only tasks.

## Project Links

- [Website](https://devspecs.com)
- [Documentation](https://docs.devspecs.com)
- [Examples](https://devspecs.com/examples)
- [Changelog](CHANGELOG.md)
- [Releases](https://github.com/devspecs-com/devspecs-cli/releases)
- [X](https://x.com/brennan_maker)
- [Reddit](https://www.reddit.com/user/bnunamak/)
- [LinkedIn](https://www.linkedin.com/in/brennan-nunamaker-30657a70)

## Development

```bash
git clone https://github.com/devspecs-com/devspecs-cli.git
cd devspecs-cli
go test ./... -count=1
go run ./cmd/ds --help
```

Before submitting a change:

```bash
gofmt -l .
go vet ./...
staticcheck ./...
go test -count=1 ./...
```

## License

[MIT License](LICENSE)
