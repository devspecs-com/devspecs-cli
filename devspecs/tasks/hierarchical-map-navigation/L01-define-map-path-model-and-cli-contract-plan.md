# Task hierarchical-map-navigation L01 Plan

## Goal
Define the map path model, naming rules, and compatible CLI contract for hierarchical `ds map`.

## Slice-Specific Scope
- Specify canonical path semantics for `ds map`, `ds map <path>`, and fuzzy/legacy scope arguments.
- Define how display names, slugs, aliases, and dot paths are derived.
- Define ambiguity behavior when a node name exists under multiple parents.
- Define JSON fields needed by agents and future slash commands.
- Decide how this coexists with `ds find` and `ds task` as the diagnostic and execution layers.

## Starting Context
- Current `ds map` produces flat subsystem blocks.
- Current `ds map Storage` expands a matching area with key files, recent signals, related areas, and caveats.
- Vana SDK status quo shows `Storage` with `Covers: Providers`, but there is no path address like `Storage.Providers`.
- I02 reclaimed `ds map` for architecture boundaries and introduced `ds recent` for the old recent-orientation flow.

## Expected Change Surface
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`

## Success Criteria
- [ ] Contract documents `ds map`, `ds map <path>`, aliases, ambiguity, not-found, and low-confidence states.
- [ ] Contract preserves existing `ds map <scope>` behavior as a compatibility path.
- [ ] Contract defines stable JSON shape for hierarchical map nodes.
- [ ] Contract includes concrete examples using `Storage`, `Storage.Providers`, and `Storage.Providers.GCS` or equivalent.
- [ ] Result records whether the slice should promote to implementation, improve naming, rework semantics, or roll back.

## Decision Gates
- Promote: implementable contract is clear, backward compatible, and product-readable.
- Improve: the direction is right but path naming, aliasing, or ambiguity copy needs another iteration.
- Rework: the design still behaves like fuzzy search instead of hierarchical navigation.
- Rollback: path hierarchy would overpromise map reliability for launch.

## Tasks
- [ ] Inspect current map command flags, output, and tests.
- [ ] Draft canonical path and alias rules.
- [ ] Draft ambiguity and not-found messages.
- [ ] Draft JSON node shape.
- [ ] Record the final contract in this result file or implementation docs before L02.
