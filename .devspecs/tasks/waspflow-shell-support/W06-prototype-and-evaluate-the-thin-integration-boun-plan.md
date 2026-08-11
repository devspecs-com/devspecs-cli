# Task waspflow-shell-support W06 Plan

## Goal
Prototype and evaluate the smallest integration boundary approved by W05.

## Entry Gate
Do not start this slice unless W05 promotes a specific contract. If existing composition is sufficient, supersede W06 and preserve the evidence.

## Candidate Experiment
- Select one ready DevSpecs slice and render its exact target/context through an existing structured output.
- Start one Waspflow isolated lane in the intended repository/worktree with that payload and a required report.
- Run the configured Waspflow verification contract.
- Attach the report, diff, and verification receipt to a human-invoked DevSpecs checkpoint without changing its decision automatically.
- Repeat one failure case: stale context, failed verification, cancelled lane, or wrong worktree.

## Ownership
- Put glue in Waspflow when it only launches/observes lanes.
- Put schema/export work in DevSpecs when it improves any orchestrator handoff.
- Avoid a DevSpecs `waspflow` command and avoid Waspflow parsing Markdown when versioned JSON is available.

## Acceptance
- The successful flow preserves task ID, slice ID, logical repo identity, effective worktree, source commit, context artifact IDs, report path, verification result, and receipt identity.
- The failure flow is explicit, bounded, and cannot produce a promoted DevSpecs checkpoint.
- Cancellation reaps child processes and does not leave an indexing worker or lane falsely authoritative.
- The prototype works on Linux, where Waspflow's tmux/runtime contract is supported.
- Any new CLI surface passes the command-surface inventory review and has evidence that composition alone was inadequate.

## Decision Gates
- Promote: the bridge removes real orchestration loss with a small, stable boundary.
- Improve: the contract is sound but ownership or UX needs one refinement.
- Supersede: existing composition is clearer and equally reliable.
- Rollback: the bridge couples lifecycle states, hides failures, or expands surface without user value.
