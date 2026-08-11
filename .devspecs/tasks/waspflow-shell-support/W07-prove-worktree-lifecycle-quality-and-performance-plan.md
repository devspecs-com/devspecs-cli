# Task waspflow-shell-support W07 Plan

## Goal
Prove worktree lifecycle, concurrency, quality, and performance regression gates.

## Dependencies
- W02-W04 promoted.
- W06 promoted or explicitly superseded.
- Rebase/merge the concurrent-index reliability work before testing parallel Waspflow lanes.

## Worktree And Concurrency Scenarios
- Index the same logical repository from the primary checkout and multiple Waspflow `--isolate` worktrees.
- Confirm repository identity deduplicates as intended and artifact growth is not multiplied by ephemeral paths.
- Remove isolated worktrees and confirm `ds prune` behavior is safe and understandable.
- Run concurrent cold/warm `map`, `find`, `recent`, `task`, and explicit index mutations against the same logical repository.
- Cancel a client during scan and verify workers exit, writes remain consistent, and later reads are trustworthy.

## Quality And Performance Matrix
- Baselines: released `v1.3.0` and the v1.4.0 release-candidate `main`; candidate: exact branch commit.
- Repositories: smoke-5, Waspflow, at least two unrelated shell-first fixtures, fat-25, and fat-100.
- Modes: independent cold home, warm repeat, self-vs-self determinism, and full-history canonical checkout.
- Commands: `scan`, `recent`, `map`, representative `find`, and representative `task`.
- Preserve raw outputs, phase timings, process exit data, artifact counts, and manual delta classifications.

## Acceptance
- No operation fails with `SQLITE_BUSY`, leaves a live indexing child, or presents an incomplete update as authoritative.
- Waspflow isolated worktrees do not create unbounded logical-repository duplication.
- All W01 quality-oracle cases are equal or better on the first cold result.
- Self-vs-self is deterministic before baseline/candidate interpretation.
- Waspflow cold median wall time is no more than 20% or 500 ms slower than W01, whichever allowance is larger; any exception requires a reviewed quality/cost decision.
- Smoke-5 plus shell fixtures runs on pull requests; fat-25 and fat-100 remain documented/manual or scheduled according to runner cost.
- `go test ./...`, race/concurrency coverage, and command-surface inventory checks pass.

## Decision Gates
- Promote: quality, lifecycle, concurrency, and performance gates all pass with classified deltas.
- Improve: no serious regression exists, but a bounded performance or fixture issue remains.
- Rework: logical identity, cancellation, or first-run quality remains ambiguous.
- Rollback: any correctness failure, weaker accepted output, or unbounded storage/process behavior appears.
