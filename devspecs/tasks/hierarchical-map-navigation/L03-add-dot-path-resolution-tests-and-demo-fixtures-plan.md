# Task hierarchical-map-navigation L03 Plan

## Goal
Add dot-path resolution tests and real-repo demo fixtures for hierarchical `ds map`.

## Slice-Specific Scope
- Add deterministic fixture coverage for canonical paths, aliases, ambiguity, not-found, and low-confidence nodes.
- Add at least one realistic OSS-style fixture inspired by the Vana SDK status quo.
- Capture raw command samples for `ds map`, `ds map Storage`, `ds map Storage.Providers`, and one provider leaf.
- Verify `--json` remains parseable and stable for agent integrations.

## Expected Change Surface
- `internal/commands/map_test.go`
- Test fixture helpers for temporary repos and git history.
- Demo/sample artifacts if the repo already has a public-safe sample location.

## Success Criteria
- [ ] Tests cover exact canonical path lookup.
- [ ] Tests cover case-insensitive or slug alias lookup.
- [ ] Tests cover ambiguous child name resolution with suggested full paths.
- [ ] Tests cover no-match and low-confidence fallback copy.
- [ ] Raw sample output is good enough to judge onboarding UX without explanation.

## Decision Gates
- Promote: tests prove the hierarchy is stable enough for launch-facing docs or demo use.
- Improve: implementation works but sample output is too noisy or unclear.
- Rework: tests reveal hierarchy depends on brittle fixture coincidences.
- Rollback: real-repo sample makes the feature look less credible than current flat map.

## Tasks
- [ ] Build or extend fixture repos with nested architecture boundaries.
- [ ] Add text-output tests for breadcrumbs and children.
- [ ] Add JSON-output tests for stable path fields.
- [ ] Capture raw samples.
- [ ] Record misses and noisy nodes in the result.
