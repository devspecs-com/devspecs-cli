# Task hierarchical-map-navigation L04 Plan

## Goal
Polish hierarchical `ds map` onboarding output, fallback states, and launch docs placement.

## Slice-Specific Scope
- Make text output easy to scan for a new engineer.
- Keep commands prominent and copy-pasteable.
- Add docs/TLDR language that positions `ds map` as onboarding/navigation and `ds task` as the execution workflow.
- Include cautious fallback copy when the hierarchy is inferred, ambiguous, or unavailable.
- Decide whether hierarchy belongs in the v1.1 launch story or as a fast-follow.

## Expected Change Surface
- `internal/commands/map.go`
- `internal/commands/tldr.go`
- `internal/commands/tldr_test.go`
- `README.md`
- `TASK_WORKFLOW_EXAMPLE.md`
- Docs site content if present in the local checkout.

## Success Criteria
- [ ] `ds map` output teaches the next navigation step without a wall of prose.
- [ ] `ds map <path>` output uses breadcrumbs, child nodes, adjacent systems, and suggested commands.
- [ ] Docs show the shortest path: `ds init`, `ds task "goal"`, and `ds map` for onboarding/trust.
- [ ] Low-confidence output is honest without sounding broken.
- [ ] Result includes a launch recommendation: ship in v1.1, beta-copy only, or defer.

## Decision Gates
- Promote: output feels like a credible onboarding feature and docs placement is clear.
- Improve: output works but copy, spacing, or suggested commands need another pass.
- Rework: the feature confuses the task-first launch story.
- Rollback: keep flat map for launch and return to hierarchy after more evidence.

## Tasks
- [ ] Review raw samples from L03.
- [ ] Tighten text output and examples.
- [ ] Update TLDR/docs only after behavior is real.
- [ ] Run focused command and docs tests.
- [ ] Record launch recommendation.
