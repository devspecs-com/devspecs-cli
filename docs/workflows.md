# Workflow Reference

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
ds task checkpoint A01 --decision promote
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
ds task checkpoint eag-c001-backend --repo ./enalytics-backend --target A01 --stage validated --decision promote --next-target A02 --next-decision promote --json
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

## Experimental Dispatch

Experimental orchestration is repository opt-in and provider-neutral:

```yaml
version: 1
integrations:
  orchestration:
    provider: waspflow
    options:
      executable: waspflow
```

`ds dispatch` consumes this setting through a DevSpecs-owned driver. The first
driver uses only stock Waspflow commands; Waspflow itself does not require a
DevSpecs patch:

```bash
ds doctor
ds dispatch task:my-task --task-repo . --cwd . --agent-provider codex
```

Dispatch waits through final receipt by default. Use `--detach` to return after
startup, `ds dispatch status <dispatch-id>` to inspect provider-neutral state,
and `ds dispatch resume <dispatch-id>` to continue monitoring and finalization.
DevSpecs keeps bounded request, handle, and receipt state under
`$DEVSPECS_HOME/dispatches/`; provider runtime state remains provider-owned.
A provider can consume the frozen `ds apply --json` input and edit its isolated
workspace, but it cannot checkpoint, promote, or otherwise mutate the DevSpecs
task lifecycle. Each driver validates its own `options`, so another unchanged
orchestrator can implement the same dispatch contract under a different
provider key. Configuration alone never launches work.

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
| `ds dispatch <task:<id>\|change:<id>> [--detach]` | Run one frozen target through an explicitly configured provider; monitor to a receipt by default. Experimental. |
| `ds task checkpoint <task-id\|target>` | Record files, tests, misses, noise, learnings, decision evidence, and next iteration. |
| `ds compose adr\|rfc\|prd "<title>"` | Create and index a repo-owned durable draft using established repository conventions. Experimental. |
| `ds hub topic/message/type/event/subscribe/pull/ack` | Share local repo-scoped coordination through discoverable topics, validated events, and pull subscriptions. Experimental; see [hub reference](hub.md). |
| `ds task slice add <task-id> "<title>" --after A01 --reason improve` | Add an A01-1-style follow-up slice after an improve/rework gate. |
| `ds task refresh <task-id>` | Recapture edited task artifacts into the local index without rewriting task docs. |
| `ds workspace init/show/change/slice/trace` | Coordinate experimental workspace-level changes, repo-local task slices, and known change/task traces. |
| `ds map` | Show architecture/system boundaries with evidence and follow-up commands. |
| `ds find <query>` | Build agent-readable packed context. |
| `ds context <id>` | Export one artifact as paste-ready agent context. |
| `ds scan` | Manually refresh or rebuild configured intent-artifact paths. |
| `ds index backup\|rebuild\|restore` | Back up, safely rebuild, or exactly restore the local SQLite index. |
| `ds prune [--dry-run] [--vacuum]` | Remove stale repository data and redundant capture revisions; compact the database explicitly with `--vacuum`. |
| `ds prune --hub --before <RFC3339> [--dry-run\|--vacuum]` | Separately delete eligible old local hub publications after a verified backup; optional compaction reports measured reclaim. Default prune does not touch hub. |
| `ds doctor [--redact] [--json]` | Inspect binary precedence, local index compatibility, writer state, repository identity, and configured-provider readiness without mutating DevSpecs state. |
| `ds config show [--json]` | Inspect effective repository configuration, including opt-in integrations. |

Most read commands support `--json`. Run `ds <command> --help` for the current
flags. Use the `ds workspace ...` form for workspace coordination.
