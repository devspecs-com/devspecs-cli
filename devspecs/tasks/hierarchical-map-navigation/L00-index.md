# Task hierarchical-map-navigation

## Task
Make `ds map` reliably navigable as a hierarchical architecture map using stable dot paths.

## Status
planned

## Series
L

## Profile
code-change

## Created At
2026-06-16T13:36:17Z

## Original Query
Plan hierarchical `ds map` navigation so users can move from `ds map` to `ds map Storage` to `ds map Storage.Providers` to `ds map Storage.Providers.GCS`.

## Repo / Workspace
- Repo: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli`
- Workspace: `C:\Users\brenn\go\src\github.com\devspecs-com\devspecs-cli\devspecs\tasks\hierarchical-map-navigation`

## Product Read
The current architecture map is already useful for first-pass repo orientation. On the Vana SDK sample, it finds areas such as `Crypto`, `Storage`, `Protocol`, `Auth`, `Platform`, and `Account`, and `ds map Storage` expands the Storage area with useful key files and recent signals.

The UX gap is navigation depth. `Storage` shows `Covers: Providers`, but users cannot naturally descend into `Storage.Providers` and then into a specific provider like `Storage.Providers.GCS` or `Storage.Providers.GoogleDrive`. A flat/fuzzy scope lookup is helpful, but it is not the onboarding-grade system map we want.

## Desired User Flow
```text
ds map
ds map Storage
ds map Storage.Providers
ds map Storage.Providers.GCS
```

Each step should feel like moving through an architecture tree:
- show breadcrumb and current node
- show purpose, boundary, evidence, key files, tests, and recent signal
- show child nodes when available
- show adjacent systems
- show the next useful `ds map`, `ds find`, and `ds task` commands

## Scope Boundaries
- Keep existing `ds map <scope>` behavior compatible while introducing path-first semantics.
- Dot paths must be stable enough for humans, agents, docs, and slash commands.
- Fuzzy matching may resolve aliases, but the canonical path displayed by the CLI should be deterministic.
- Avoid broad workspace support in this track. Use project-root map quality as the target.
- Do not promise perfect architecture discovery. Use confidence and caveats when hierarchy is inferred.

## Task Slices
- L01: Define map path model, naming rules, and compatible CLI contract. Plan: `L01-define-map-path-model-and-cli-contract-plan.md`. Result: `L01-define-map-path-model-and-cli-contract-result.md`.
- L02: Build hierarchy generation from boundary areas, children, aliases, and evidence. Plan: `L02-build-hierarchy-generation-from-boundary-areas-plan.md`. Result: `L02-build-hierarchy-generation-from-boundary-areas-result.md`.
- L03: Add dot-path resolution tests and real-repo demo fixtures. Plan: `L03-add-dot-path-resolution-tests-and-demo-fixtures-plan.md`. Result: `L03-add-dot-path-resolution-tests-and-demo-fixtures-result.md`.
- L04: Polish onboarding output, fallback states, and docs placement. Plan: `L04-polish-onboarding-output-and-fallback-states-plan.md`. Result: `L04-polish-onboarding-output-and-fallback-states-result.md`.

## Key Decisions To Resolve
- Canonical path casing: preserve display names (`Storage.Providers.GoogleDrive`) while accepting lowercase/slug aliases.
- Provider naming: decide whether `gcs` aliases to `GCS`, `google-drive` aliases to `GoogleDrive`, or both can exist as children under `Storage.Providers`.
- Ambiguity UX: when `Providers` exists under multiple parents, require or suggest the full path instead of silently picking a weak match.
- JSON contract: include stable path, parent path, children, aliases, confidence, boundary paths, evidence, and suggested commands.
- Launch placement: if L02/L03 show strong behavior, top-level `ds map` becomes a flagship onboarding command; if not, keep hierarchy language cautious until follow-up.

## Decision Gates
- Promote from L01 to L02 when the CLI contract is explicit enough to implement without breaking existing `ds map <scope>` users.
- Improve L01 if path naming, aliasing, or ambiguity handling still feels arbitrary.
- Rework L01 if dot paths collapse into fuzzy search rather than stable navigation handles.
- Roll back the hierarchy plan if real repos cannot produce credible child boundaries without misleading users.

## Launch Relevance
High. A hierarchical map would make DevSpecs feel much more like an instant repo onboarding tool: `ds task` remains the main work loop, while `ds map` becomes the fastest way to understand where work lives.
