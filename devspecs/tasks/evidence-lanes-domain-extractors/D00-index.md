# Task evidence-lanes-domain-extractors

## Task
Evidence lanes and domain extractors

## Status
packed

## Series
D

## Profile
greenfield

## Created At
2026-06-12T07:40:19Z

## Original Query
Evidence lanes and domain extractors

## Track Intent
This is the mid-term domain-evidence track. It generalizes the existing intent mining and graph machinery without inventing a new graph storage model for every workflow.

The core idea: profiles opt into named evidence lanes, and built-in extractors produce concepts, mentions, facts, artifact edges, diagnostics, and pack-admission signals. Security, API contract, privacy/compliance, incident, migration, performance, and UX work can then get better context without hard-coding one giant ontology.

## Timing
Mid-term v2, after local profile templates exist. Evidence lanes should serve profiles; profiles should not wait for arbitrary user-defined extraction.

## Product Decisions
- Start with a registry for lane metadata and admission behavior: `can_promote_to_pack`, `support_only`, or `diagnostic_only`.
- Ship built-in lanes before exposing user-authored extractors.
- Security and API contracts are good first candidates because they need graph-like relationships beyond source/test companions.
- Diagnostics must explain which lanes promoted, supported, demoted, or merely warned about context.

## Non-Goals
- Do not replace the current concepts, mentions, artifact edges, checkpoint facts, source manifest, or git receipt storage model.
- Do not let marketplace profiles execute custom extractors in the CLI.
- Do not make broad body-window retrieval or vector rerank the core evidence mechanism.

## Decision Gates
- Promote D01 when lane metadata can be reviewed without reading retrieval internals.
- Promote D02 when extractors can emit evidence without owning ranking policy.
- Promote D03 only with deterministic fixtures showing security/API lanes improve context without noisy overreach.
- Rework if the lane abstraction becomes a vague plugin system instead of a small evidence contract.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/evidence-lanes-domain-extractors`

## Resources
- `task.json`
- `D01-define-evidence-lane-registry-for-profile-opt-in-plan.md`
- `D01-define-evidence-lane-registry-for-profile-opt-in-result.md`
- `D02-add-extractor-contract-for-domain-facts-concepts-plan.md`
- `D02-add-extractor-contract-for-domain-facts-concepts-result.md`
- `D03-ship-built-in-security-and-api-contract-lanes-be-plan.md`
- `D03-ship-built-in-security-and-api-contract-lanes-be-result.md`
- `D04-add-diagnostics-that-explain-which-lanes-influen-plan.md`
- `D04-add-diagnostics-that-explain-which-lanes-influen-result.md`

## Task Slices
- D01: Define evidence lane registry for profile opt-in and pack admission behavior. Plan: `D01-define-evidence-lane-registry-for-profile-opt-in-plan.md`. Result: `D01-define-evidence-lane-registry-for-profile-opt-in-result.md`.
- D02: Add extractor contract for domain facts concepts mentions and artifact edges. Plan: `D02-add-extractor-contract-for-domain-facts-concepts-plan.md`. Result: `D02-add-extractor-contract-for-domain-facts-concepts-result.md`.
- D03: Ship built-in security and API contract lanes before user-defined extraction. Plan: `D03-ship-built-in-security-and-api-contract-lanes-be-plan.md`. Result: `D03-ship-built-in-security-and-api-contract-lanes-be-result.md`.
- D04: Add diagnostics that explain which lanes influenced pack promotion support or demotion. Plan: `D04-add-diagnostics-that-explain-which-lanes-influen-plan.md`. Result: `D04-add-diagnostics-that-explain-which-lanes-influen-result.md`.

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

- `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence
- `internal/retrieval/pack_negative_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence
- `internal/scan/evidence_graph_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence
- `internal/scan/workstream_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence
- `internal/store/evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/commands/find_pack_scout_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence; `internal/retrieval/pack_negative_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence; `internal/scan/evidence_graph_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence; `internal/scan/workstream_evidence_test.go` - on-disk path matched task terms but was not in the indexed candidate set: evidence

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
Use `D01-define-evidence-lane-registry-for-profile-opt-in-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `D01-define-evidence-lane-registry-for-profile-opt-in-result.md` or `ds task checkpoint`.
