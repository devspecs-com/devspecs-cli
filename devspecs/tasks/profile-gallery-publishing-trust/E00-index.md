# Task profile-gallery-publishing-trust

## Task
Profile gallery and publishing trust

## Status
packed

## Series
E

## Profile
greenfield

## Created At
2026-06-12T07:40:31Z

## Original Query
Profile gallery and publishing trust

## Track Intent
This is the later trust and distribution track for sharing workflow profiles. It should only proceed after local profile support proves repeatable value.

The posture is explicit and opt-in: curated free profile gallery first, inspectable files, pinned versions, no background sync, and no automatic publishing of user specs. This track also keeps profile sharing separate from any specs.place story so DevSpecs can keep owning the local trust wedge.

## Timing
Later v2+. Do not start before C track has a working local profile system and at least one first-party profile with real dogfood value.

## Product Decisions
- A gallery should feel like examples and reusable workflow packs, not an app store dependency.
- Profile install/update should be explicit, auditable, and version-pinned.
- specs.place should remain a separate explicit publishing/provenance story, not a background profile channel.
- Marketplace timing should be decided from adoption evidence, not launch ambition.

## Non-Goals
- Do not build background upload or cloud sync.
- Do not auto-publish specs or profiles.
- Do not imply published artifacts are training data.
- Do not monetize or commercialize this story before OSS trust is established.

## Decision Gates
- Promote E01 when the gallery contract is safe enough to document publicly.
- Promote E02 only if install/update/audit UX is explicit and reversible.
- Block E03 if it blurs local DevSpecs trust with specs.place publishing.
- Rework E04 if marketplace language makes the CLI feel commercially dependent too early.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/profile-gallery-publishing-trust`

## Resources
- `task.json`
- `E01-define-curated-free-profile-gallery-contract-wit-plan.md`
- `E01-define-curated-free-profile-gallery-contract-wit-result.md`
- `E02-add-explicit-profile-install-update-and-audit-ux-plan.md`
- `E02-add-explicit-profile-install-update-and-audit-ux-result.md`
- `E03-separate-profile-sharing-from-specs-place-publis-plan.md`
- `E03-separate-profile-sharing-from-specs-place-publis-result.md`
- `E04-decide-marketplace-timing-after-first-party-prof-plan.md`
- `E04-decide-marketplace-timing-after-first-party-prof-result.md`

## Task Slices
- E01: Define curated free profile gallery contract with pinned versions and inspectable files. Plan: `E01-define-curated-free-profile-gallery-contract-wit-plan.md`. Result: `E01-define-curated-free-profile-gallery-contract-wit-result.md`.
- E02: Add explicit profile install update and audit UX without background sync. Plan: `E02-add-explicit-profile-install-update-and-audit-ux-plan.md`. Result: `E02-add-explicit-profile-install-update-and-audit-ux-result.md`.
- E03: Separate profile sharing from specs.place publishing so local trust stays primary. Plan: `E03-separate-profile-sharing-from-specs-place-publis-plan.md`. Result: `E03-separate-profile-sharing-from-specs-place-publis-result.md`.
- E04: Decide marketplace timing after first-party profiles prove repeatable value. Plan: `E04-decide-marketplace-timing-after-first-party-prof-plan.md`. Result: `E04-decide-marketplace-timing-after-first-party-prof-result.md`.

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

- `internal/profiles/profiles_test.go` - on-disk path matched task terms but was not in the indexed candidate set: profile

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/profiles/profiles_test.go` - on-disk path matched task terms but was not in the indexed candidate set: profile

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
Use `E01-define-curated-free-profile-gallery-contract-wit-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `E01-define-curated-free-profile-gallery-contract-wit-result.md` or `ds task checkpoint`.
