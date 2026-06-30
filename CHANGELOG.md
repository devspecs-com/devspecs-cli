# Changelog

## Unreleased

- Added experimental workspace coordination commands for umbrella repos:
  `ds workspace change create`, `ds workspace slice create`, and
  `ds workspace trace`. Top-level `ds change`, `ds slice`, and `ds trace`
  remain hidden compatibility aliases, with regression coverage for alias help
  and dispatch.
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

## v1.1.0 - draft

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
