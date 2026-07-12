# Changelog

## Unreleased

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
