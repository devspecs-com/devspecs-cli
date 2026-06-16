# Task hierarchical-map-navigation L02 Plan

## Goal
Build hierarchy generation from existing boundary areas, child paths, aliases, and evidence.

## Slice-Specific Scope
- Convert flat boundary areas into a tree of map nodes.
- Infer child nodes from path structure, package/module boundaries, `Covers` values, tests, imports, docs, and recent commits.
- Preserve canonical parent/child relationships in text and JSON output.
- Keep confidence explicit when a child is inferred from path shape rather than stronger evidence.
- Ensure `ds map Storage.Providers` feels like a focused system map, not just a narrower file list.

## Expected Change Surface
- `internal/commands/map.go`
- `internal/commands/map_test.go`
- Supporting map/boundary helpers if already split out.

## Success Criteria
- [ ] `ds map` lists top-level subsystems and shows obvious child hints.
- [ ] `ds map Storage` shows breadcrumb, children, key files, related areas, and follow-up commands.
- [ ] `ds map Storage.Providers` resolves a child area when path evidence exists.
- [ ] Provider-like leaf nodes can be resolved by canonical path or documented alias when confidence is high enough.
- [ ] Low-confidence children are labeled as inferred and do not masquerade as authoritative system boundaries.

## Decision Gates
- Promote: hierarchy is useful in this repo and at least one real OSS-style fixture without misleading child nodes.
- Improve: hierarchy works but labels, aliases, or child ordering are noisy.
- Rework: generated children are mostly folder echoes with little onboarding value.
- Rollback: hierarchy makes the architecture map less trustworthy than the flat output.

## Tasks
- [ ] Inspect the current boundary detection structures.
- [ ] Add an internal map-node model if needed.
- [ ] Implement child derivation and canonical path generation.
- [ ] Update text output for breadcrumbs and children.
- [ ] Update JSON output for path, parent, children, aliases, and confidence.
- [ ] Run focused map tests.
