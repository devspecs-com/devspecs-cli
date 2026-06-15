# Task brownfield-active-intent-ranking

## Task
Brownfield active intent ranking and scoped find packs

## Status
packed

## Series
F

## Profile
greenfield

## Created At
2026-06-15T15:09:12Z

## Original Query
Brownfield active intent ranking and scoped find packs

## Track Intent
This is the pre-launch retrieval-quality track created from ScopeLab dogfood feedback. It covers the cases where `ds find` is technically defensible but product-misleading because stale or blocked historical plans outrank current owner decision records.

The concrete failure mode: before EV artifacts existed, a query for `epoch 4 external validity bridge` surfaced blocked D4.2-era work as primary and downgraded current indexes as stale. After EV artifacts existed, `ds find` became useful, but a tangential historical plan still appeared in the pack for an EV-R1 query.

## Timing
Pre-launch or early patch. This is more urgent than broad evidence-lane extensibility because it protects the brownfield trust story for real plan-heavy repos.

## Product Decisions
- Current owner decision records, north-star active-phase docs, and `Status: next` plans should beat blocked, closed, superseded, or stale epics.
- Exact plan IDs, track IDs, and slice IDs should create a narrow retrieval mode: direct artifact, direct neighbors, and explicit references first; historical analogs later or capped.
- Downgraded historical context can remain visible, but it must not look operationally primary.
- Do not treat `ds find` as complete truth. It should route agents to the right canonical docs.

## Non-Goals
- Do not build a full planning-graph replacement.
- Do not solve this with broad vector reranking or bigger body windows.
- Do not hide all historical context; just keep it from inflating the operational pack.

## Decision Gates
- Promote F01 when active decision docs reliably outrank blocked/superseded plans in deterministic fixtures.
- Promote F02 when plan-ID queries keep direct neighbors and suppress tangential historical plans.
- Improve if labels are right but ordering still sends agents to stale lanes.
- Rework if the scoring becomes too repo-specific or requires ScopeLab-only conventions.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/brownfield-active-intent-ranking`

## Resources
- `task.json`
- `F01-boost-owner-decision-records-active-phase-docs-a-plan.md`
- `F01-boost-owner-decision-records-active-phase-docs-a-result.md`
- `F02-tighten-exact-plan-id-and-track-id-find-packs-so-plan.md`
- `F02-tighten-exact-plan-id-and-track-id-find-packs-so-result.md`
- `F03-add-fixtures-for-brownfield-recovery-before-arti-plan.md`
- `F03-add-fixtures-for-brownfield-recovery-before-arti-result.md`

## Task Slices
- F01: Boost owner decision records active phase docs and Status next plans above blocked or superseded epics. Plan: `F01-boost-owner-decision-records-active-phase-docs-a-plan.md`. Result: `F01-boost-owner-decision-records-active-phase-docs-a-result.md`.
- F02: Tighten exact plan ID and track ID find packs so direct neighbors beat tangential historical plans. Plan: `F02-tighten-exact-plan-id-and-track-id-find-packs-so-plan.md`. Result: `F02-tighten-exact-plan-id-and-track-id-find-packs-so-result.md`.
- F03: Add fixtures for brownfield recovery before artifacts exist and after current decision docs exist. Plan: `F03-add-fixtures-for-brownfield-recovery-before-arti-plan.md`. Result: `F03-add-fixtures-for-brownfield-recovery-before-arti-result.md`.

## Relevant Map Areas
No strong map area was inferred from the initial pack.

## Likely Primary Files
None found in the initial preflight.

## Likely Tests
None found in the initial preflight.

## Likely Docs / Plans / Config
None found in the initial preflight.

## Supporting Context
None found in the initial preflight.

## Related Git Receipts
None found from packed paths.

## Noise Risks
None found in the initial preflight.

## Freshness Warnings
These on-disk paths match the task wording but were not present in the indexed candidate set. Treat them as stale-index risk, not proof that the initial pack is wrong.

- `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_pack_scout_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_pack_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_source_manifest_consumption_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_source_manifest_recovery_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find
- `internal/commands/find_source_pack_mode_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/find_pack_companions_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find; `internal/commands/find_pack_presentation_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find; `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find; `internal/commands/find_pack_scout_test.go` - on-disk path matched task terms but was not in the indexed candidate set: find

## Known Knowns
- The task workspace was created, but the initial evidence is sparse.

## Known Unknowns
- Primary implementation surface is unknown.
- Relevant tests may be missing from the initial pack.
- Task-related on-disk paths may be missing from the indexed candidate set.
- Pack completeness is not high; verify the working set before committing to implementation scope.

## Confidence Summary
- Primary file confidence: low
- Test coverage confidence: low
- Docs/config coverage confidence: low
- Git receipt confidence: low
- Noise risk: low
- Pack completeness: low

Why:
- no clear primary implementation file was found
- test companion coverage was not evident from the initial pack

Agent instruction:
Use the evidence to define the first bounded planning artifact, evaluation signal, and next-slice decision before implementation scope expands.

## Suggested Starting Slice
Use `F01-boost-owner-decision-records-active-phase-docs-a-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `F01-boost-owner-decision-records-active-phase-docs-a-result.md` or `ds task checkpoint`.
