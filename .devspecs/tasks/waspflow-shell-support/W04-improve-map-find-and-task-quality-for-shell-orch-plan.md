# Task waspflow-shell-support W04 Plan

## Goal
Improve `map`, `find`, `task`, and `recent` quality for shell orchestration repositories.

## Scope
Use the substrate added by W02-W03. Do not add Waspflow path-name bonuses or weaken cold-start indexing to hit timing targets.

## Waspflow Quality Oracle
- `map` should expose coherent boundaries for CLI/lane lifecycle, provider adapters, verification and receipts, selection/escalation/billing, worktree/project safety, events/observation, and fan-in/capture.
- `find "worker lane lifecycle"` should include `bin/waspflow` plus relevant lane/artifact implementation and behavior tests.
- `find "provider orchestration"` should include provider adapters, dispatch, event handling, and verification coverage.
- `find "verification escalation"` should include verify/escalation implementation, receipt behavior, and tests, with design docs as support rather than the only role.
- The task probe should return implementation, tests, constraints, and a bounded stop line.
- `recent` must preserve or improve the five useful Git-derived topics from W01 and may enrich them with indexed boundaries.

## Work
- Feed shell symbols, roles, and relationships into existing map/retrieval/task scoring.
- Adjust generic boundary naming only when the same rule improves unrelated shell-first fixtures.
- Keep design/history context subordinate to current implementation for implementation queries.
- Classify every output delta using the existing manual review workflow.

## Acceptance
- Each implementation query includes at least one primary implementation artifact and one behavior-test artifact when a relevant test exists.
- Role diversity is greater than 1 for behavior queries.
- Waspflow map boundaries match the reviewed oracle without requiring exact wording.
- The same rules improve or preserve results on at least two pinned, unrelated shell-first repositories.
- No `recent`, `map`, `find`, or `task` first-result regression in smoke-5 or the reviewed canonical corpus.
- No special case checks for repository name `waspflow` or owner `tnunamak`.

## Decision Gates
- Promote: quality gains generalize and all first-result preservation gates pass.
- Improve: Waspflow improves but one cross-repo delta remains unclassified.
- Rework: improvements depend on repository-specific vocabulary or path bonuses.
- Rollback: any accepted baseline result becomes weaker.
