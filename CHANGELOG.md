# Changelog

## Unreleased

## v1.4.0 - 2026-08-12

- Added experimental repo-first named execution threads with `ds thread set`,
  `ds thread remove`, owner-wide status, dependency joins, and explicit `ds
  apply <owner> --thread <key>` prompts. Tasks remain repo-owned by default;
  only tasks explicitly linked to a workspace change join its cross-repo graph.
- Hid `ds task next` from normal help while retaining callable compatibility
  for legacy linear tasks and one unambiguous lane. Multi-lane status and apply
  paths never choose the first runnable lane and instead print exact thread and
  apply commands.
- Made immutable checkpoint JSON events the durable lifecycle authority for
  named-thread scheduling, with YAML graph definitions and a rebuildable SQLite
  projection that reconstructs equivalent cold status after prune. No JSONL or
  remote service is required.
- Serialized thread graph mutations with repo and linked child-task checkpoint
  publication, preserving every event and preventing a workspace graph edit
  from overlooking newly started work. Blocked lanes now print an exact
  follow-up or conflict-resolution command.
- Preserved repo-owned durability closeout after a cross-repo graph completes:
  `ds apply <linked-task> --repo <child-repo>` returns that task's `A00`-style
  closeout, while applying the completed workspace change remains terminal.
- Added experimental `ds compose adr|rfc|prd` to create indexed, repo-owned
  durable drafts outside the DevSpecs task corpus while reusing established
  document directories, numbering, and ADR conventions. Composed files remain
  authoritative through index rebuild/prune operations, and workspace callers
  route ownership explicitly with `--repo` rather than a parallel workspace
  compose command. Conventional ADR paths retain single-adapter ownership after
  rebuild so retrieval does not return duplicate records for one file.
- Added ADR templates for all formats compared by adr.zone: Nygard, MADR full
  and minimal, Y-Statement, Outcome-First, and the ISO 42010 Companion.
- Added a one-time durability closeout after full task-track implementation
  slices finish. New full tracks must explicitly record that no durable document
  is needed, link completed ADR/RFC/PRD artifacts, or defer the record to a
  named target; compact `--quick` tasks and existing manifests keep their prior
  lifecycle behavior.

- Changed concurrent index mutations to queue behind one bounded writer lease so
  overlapping `scan`, `map`, `find`, `recent`, and `task` operations do not fail
  with transient SQLite lock errors.
- Added visible 10-minute auto-index and 30-minute explicit-scan deadlines, with
  signal cancellation that reaps active Git subprocesses instead of leaving
  long-running workers behind.
- Changed `ds task` creation to stage and validate complete workspaces before
  atomic publication. Interrupted preflight no longer leaves an empty final
  task directory, forced retries safely replace remnants, and failed success
  output now returns nonzero with the durable task ID and path in the error.
- Changed indexed task creation, slice addition, checkpoints, and legacy
  lifecycle mutations to verify local database compatibility before writing
  repository files. An older CLI against a newer derived index now reports the
  database, schema, executable, recovery, and that no repository files were
  written instead of leaving partial task state that retries can duplicate.
- Added `ds prune`, with `--dry-run`, JSON output, and explicit `--vacuum`
  compaction, to remove index data for repository roots that no longer exist
  and collapse redundant consecutive capture revisions without losing content
  transitions.
- Added bounded prune/vacuum progress on stderr for long human-mode maintenance
  waits while preserving clean JSON stdout.
- Changed Git repository identity from physical path alone to canonical remote
  plus root commit, with root aliases so ephemeral worktrees reuse one logical
  repository index instead of duplicating all artifacts.
- Scoped relative artifact source identities to their logical repository so
  common paths such as `AGENTS.md` cannot collide across unrelated repos.
- Added conservative repair for legacy cross-repository source ownership,
  including source-ownership indexes that keep fat-index prune checks bounded.
- Changed repeated `ds capture` calls so unchanged content refreshes metadata
  without appending another revision.
- Changed `ds update` detection so a manually installed `/usr/local/bin/ds`
  binary is not reported as Homebrew without stronger Homebrew path evidence.
- Changed top-level command failure rendering to print actionable errors once
  without an unrelated usage dump.

## v1.3.0 - 2026-07-12

- Added `ds task --quick` for compact one-off task workspaces and hid the older
  `ds task quick` form from normal help as compatibility surface.
- Changed `ds apply` with no target to resolve the unambiguous next slice and
  added next-target guidance to `ds task status`; help/docs now present
  argument-free `ds apply` as the happy path.
- Added `ds task slice add --after <slice> --reason <gate>` for A01-1-style
  follow-up slices and hid `ds task iteration` from normal help.
- Hid legacy task lifecycle shortcuts (`prompt`, `finish`, `decide`, `start`,
  and `sync`) from normal help while keeping compatibility paths callable and
  redirecting users toward `ds apply`, `ds task checkpoint`, and
  `ds task refresh`.

## v1.2.0 - 2026-07-12

DevSpecs v1.2 expands the CLI from repo-local task execution into workspace-aware
coordination, stronger regression discipline, better first-run map/recent
quality, and faster cold indexing.

Workspace and task coordination:

- Added experimental workspace coordination commands for umbrella repos:
  `ds workspace change create`, `ds workspace slice create`, and
  `ds workspace trace`. Top-level `ds change`, `ds slice`, and `ds trace`
  remain hidden compatibility aliases, with regression coverage for alias help
  and dispatch.
- Added `ds ws` as a built-in shortcut for `ds workspace` and introduced
  `specs/cli-surface.yaml` as the first canonical CLI surface audit artifact.
- Added explicit `--repo` routing for repo-local task and apply flows so agents
  can work from an umbrella root without writing task artifacts into the wrong
  repo.
- Added workspace trace output for known change/task IDs, including per-slice
  lifecycle status and aggregate workspace change completeness.
- Added `ds task checkpoint --draft` to preview checkpoint markdown, structured
  JSON evidence, and result append text without mutating task lifecycle state.
- Added `ds task checkpoint --from-git` to populate edited-file evidence from
  current git status/diff paths.
- Added `ds task checkpoint --run-log` to ingest explicit test/build/typecheck
  run logs as actual run evidence plus bounded structured output.
- Changed checkpoint result appends to convert the initial instruction section
  into `## Checkpoint History` after the first real checkpoint.

Quality and activation regression infrastructure:

- Added activation regression support for baseline-vs-candidate binary
  comparisons, canonical skinny/fat/full-history repo manifests, structured
  `ds map` comparison, and self-vs-self determinism checks.
- Added a cross-command cold activation gate for substrate consumers so
  `recent`, `map`, `find`, and `task quick` can be checked together before
  promoting indexing or output changes.
- Added a reviewed fat-100 activation quality baseline that layers strict
  v1.1.0 comparisons with accepted manual/automatic delta classifications,
  allowing future runs to distinguish true regressions from already-reviewed
  same-or-better output changes.
- Archived and documented canonical regression sets so future performance and
  quality work can start from the same small, fat-25, and fat-100 repo sets
  instead of rediscovering old manifests.

Map and recent quality:

- Improved `ds map` first-run behavior so default `map` builds the required
  local substrate before producing handoff commands instead of returning weaker
  index-missing output.
- Improved `ds map` boundary ranking and handoff suggestions with indexed
  packability checks, source/test balance signals, cached map output, and
  stricter structured comparison gates.
- Improved `ds recent` topic quality for noisy public repos such as FastAPI by
  merging overlapping recent work, demoting generic maintenance/setup topics,
  preserving specific README/spec/version-manifest topics, and surfacing
  system-boundary hints when available.

Cold indexing performance and UX:

- Added a fresh-index writer path for empty/cold repositories with batched row
  writes, deferred FTS updates, transaction-aware scans, and avoided per-file
  authored-at lookups where first-run semantics are unchanged.
- Reduced cold scan cost in source manifest, test-case, source companion, and
  evidence graph paths, including parallel source/test discovery and lower
  evidence mention construction overhead.
- Added phase timing and benchmark output for cold first-index runs, including
  source manifest, evidence graph, DB/write, and cross-command activation
  telemetry.
- Improved non-quiet cold-start progress output for `recent`, `map`, `find`,
  `task`, and scan-backed auto-indexing. Default progress now reports
  high-level checkpoints on stderr, while `--verbose` exposes detailed
  discovery, extraction, persistence, evidence graph, source manifest, and
  search-index phases without polluting result stdout or JSON output.

CLI surface polish:

- Clarified `ds scan` help copy so it describes a repository intent/source/test
  rescan instead of only specs, plans, and ADRs.

## v1.1.0 - 2026-06-17

DevSpecs v1.1 centers the launch story on bounded task execution for AI coding
agents.

- `ds task` is the primary workflow for creating packed, repo-grounded task
  slices with plan/result artifacts, checkpoints, and decision gates.
- `ds apply` emits the next bounded one-slice agent prompt without mutating task
  state.
- `ds init` can generate Codex, Cursor, Claude, and Windsurf adapter files for
  `ds task` and `ds apply`.
- `ds tldr` provides LLM-oriented quickstarts grouped by setup, hotfix, epic,
  incident, brownfield recovery, handoff, and repo deep dive workflows.
- `ds find` now builds packed context by default; use `ds find --plain` for the
  older flat result list.
- `ds map` focuses on architecture/system boundaries, while `ds recent` covers
  recently active local git topics.
- `ds update` reports the active binary, likely install source, latest release
  status, and recommended update command.

Known launch caveats:

- `ds apply` is prompt-only in v1.1; it does not launch an external coding
  agent.
- Generated agent adapter files are thin wrappers over the local CLI, not a
  hosted service.
- `ds adopt` is planned, not included in v1.1.0.
- Workspace coordination is experimental dogfood surface and may be renamed or
  consolidated before public launch.
