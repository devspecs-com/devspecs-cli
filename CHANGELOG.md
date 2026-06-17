# Changelog

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
- `ds adopt` and richer workspace support are planned, not included in v1.1.0.
