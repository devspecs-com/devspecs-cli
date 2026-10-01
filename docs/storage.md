# Storage And Privacy

| Location | Role | Commit? |
| --- | --- | --- |
| `~/.devspecs/devspecs.db` | Local SQLite index and cache. | No. |
| `~/.devspecs/hub.sqlite` | Local, authoritative repo and opt-in home-global hub topics, messages, events, subscription cursors, and cooperative leases. Not reconstructed from the index. | No. |
| `~/.devspecs/hub-prune-*.sqlite` | Verified snapshot made before an explicit hub history prune; managed by the CLI. | No. |
| `~/.devspecs/backups/index/` | Verified manual and automatic index recovery snapshots. | No. |
| `~/.devspecs/dispatches/<dispatch-id>/` | Frozen dispatch requests, provider handles, and normalized receipts. | No. |
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
workspaces optionally coordinate explicitly linked repositories, and hub
holds local, short-lived coordination. The default prune maintains derived
index state without deleting repository artifacts.

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

Hub history has a separate, explicit retention path:

```bash
ds prune --hub --before 2026-09-01T00:00:00Z --dry-run
ds prune --hub --before 2026-09-01T00:00:00Z
```

Each run deletes at most 10,000 eligible old publications after a verified
SQLite backup. Old unpinned message heads and whole old event correction
chains are eligible; pinned heads, newer chains, event schemas, replay-gap
records, and idempotency tombstones remain. Repeat while the report says
`more` to continue. A group above 10,000 publications is not yet supported.
A normal `ds prune` never touches hub.
Hub row deletion reuses SQLite pages; it does not by itself shrink the live
file. Add `--vacuum` to the non-dry-run hub command to compact explicitly and
report measured before/after bytes. It needs extra disk headroom and may wait
on other readers/writers. Use [the hub reference](hub.md) for current retention
limitations.

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
export DEVSPECS_TELEMETRY=0
```

or:

```bash
export DS_TELEMETRY=0
```

Use `DEVSPECS_TELEMETRY=debug` to print the would-send event to stderr.
