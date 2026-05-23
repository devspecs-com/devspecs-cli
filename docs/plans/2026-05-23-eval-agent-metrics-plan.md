# Eval Agent Metrics Plan

Date: 2026-05-23

## Goal

Emit agent-oriented retrieval metrics directly from `ds eval` so real-repo eval runs automatically include more diagnostic signal than raw artifact precision.

This should cover:

- split lane metrics for docs/plans, test cases, code comments, other source context, and packed-section overlays
- graded relevance for exact must/helpful/background hits, same-file source/test clusters, unlabeled hits, and hard negatives
- agent-shaped metrics such as must-hit-at-k, first useful/must rank, and sufficiency under token budgets

## Implementation

- Add eval-harness result schema fields:
  - top-level `agent_metrics`
  - top-level `lane_metrics`
  - top-level `metric_notes`
  - per-case `agent_metrics`
  - per-case `artifact_grades`
- Compute these during `evalharness.Run` after exact artifact metrics and context sufficiency are known.
- Include the new fields in first-index retrieval reports.
- Extend the real50 runner aggregate to carry the new `ds eval` fields when a future CLI build emits them.

## Auditable Success Criteria

- `ds eval <fixture> --json --no-save` includes `agent_metrics`, `lane_metrics`, `metric_notes`, per-case `agent_metrics`, and per-case `artifact_grades`.
- Text output includes must-hit, token-budget sufficiency, lane metrics, and per-case graded precision.
- First-index report JSON includes retrieval `agent_metrics` and `lane_metrics`.
- Existing exact precision/recall/sufficiency fields remain present and unchanged in meaning.
- The real50 runner remains backward-compatible with older result JSONs that do not contain these fields.
- Focused eval harness and eval command tests pass.
- A CLI JSON smoke confirms the new fields are emitted without saving or running a full real50 eval.
