# Task workflow-profile-templates

## Task
User-defined workflow profiles and templates

## Status
packed

## Series
C

## Profile
greenfield

## Created At
2026-06-12T07:40:08Z

## Original Query
User-defined workflow profiles and templates

## Track Intent
This is the near-term v2 customization track. It captures the path from hard-coded profiles (`code-change`, `greenfield`) toward safe user-defined workflow profiles for tasks like UX/product UI, incident response, security review, and evaluation work.

The first version should be declarative and local: templates, required sections, prompt rules, evidence requirements, and decision gates. It should not execute arbitrary hooks, install remote code in the background, or let profile authors redefine retrieval internals.

## Timing
Near-term v2, after launch trust fixes and before marketplace or domain extractor work.

## Product Decisions
- Custom profiles should start as repo/user-local files, not a remote marketplace dependency.
- Profiles can shape task docs, prompts, result requirements, and acceptance gates.
- The UX/product-ui profile is the proof profile because dogfooding showed that visual work needs critique capture, first-viewport contracts, screenshot gates, and human acceptance.
- Profiles may request evidence lanes once the D track exists, but should not define arbitrary graph logic themselves.

## Non-Goals
- Do not build a profile publishing marketplace in this track.
- Do not add executable hooks or background updates.
- Do not make every task profile first-party before the schema has proven itself.

## Decision Gates
- Promote C01 when the profile schema is simple enough to explain in docs and strict enough to keep generated artifacts parseable.
- Promote C03 only if the UX profile demonstrably improves a real dogfood flow versus the generic greenfield profile.
- Improve if the schema makes common templates easy but edge cases clumsy.
- Rework if profile customization fragments task artifact shape so much that `show`, `prompt`, `status`, or `sync` degrade.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:/Users/brenn/go/src/github.com/devspecs-com/devspecs-cli/devspecs/tasks/workflow-profile-templates`

## Resources
- `task.json`
- `C01-define-safe-local-profile-schema-for-task-templa-plan.md`
- `C01-define-safe-local-profile-schema-for-task-templa-result.md`
- `C02-support-repo-and-user-profile-discovery-without-plan.md`
- `C02-support-repo-and-user-profile-discovery-without-result.md`
- `C03-add-first-party-ux-product-ui-profile-as-the-pro-plan.md`
- `C03-add-first-party-ux-product-ui-profile-as-the-pro-result.md`
- `C04-document-profile-authoring-and-migration-from-ha-plan.md`
- `C04-document-profile-authoring-and-migration-from-ha-result.md`

## Task Slices
- C01: Define safe local profile schema for task templates gates prompts and evidence requirements. Plan: `C01-define-safe-local-profile-schema-for-task-templa-plan.md`. Result: `C01-define-safe-local-profile-schema-for-task-templa-result.md`.
- C02: Support repo and user profile discovery without executable hooks or auto-update. Plan: `C02-support-repo-and-user-profile-discovery-without-plan.md`. Result: `C02-support-repo-and-user-profile-discovery-without-result.md`.
- C03: Add first-party UX product-ui profile as the proof profile. Plan: `C03-add-first-party-ux-product-ui-profile-as-the-pro-plan.md`. Result: `C03-add-first-party-ux-product-ui-profile-as-the-pro-result.md`.
- C04: Document profile authoring and migration from hard-coded profiles. Plan: `C04-document-profile-authoring-and-migration-from-ha-plan.md`. Result: `C04-document-profile-authoring-and-migration-from-ha-result.md`.

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

- `internal/profiles/profiles_test.go` - on-disk path matched task terms but was not in the indexed candidate set: profiles
- `internal/userident/userident_test.go` - on-disk path matched task terms but was not in the indexed candidate set: user

## Risk Cards
Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.

- On-disk paths matched the task but were not indexed [medium, freshness]
  Agent check: Inspect the warned files or refresh the index before trusting missing context.
  Evidence: `internal/profiles/profiles_test.go` - on-disk path matched task terms but was not in the indexed candidate set: profiles; `internal/userident/userident_test.go` - on-disk path matched task terms but was not in the indexed candidate set: user

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
Use `C01-define-safe-local-profile-schema-for-task-templa-plan.md` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.

## Agent Preflight Checklist
- [ ] Treat predicted files as evidence, not required edit targets.
- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.
- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.
- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `C01-define-safe-local-profile-schema-for-task-templa-result.md` or `ds task checkpoint`.
