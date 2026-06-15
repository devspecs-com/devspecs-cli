# DevSpecs Task Roadmap

This folder holds versionable intent artifacts for launch-facing and post-launch DevSpecs CLI work.

## Current Tracks

| Series | Track | Timing | Purpose |
| --- | --- | --- | --- |
| A | `scanless-workflow-ux` | Launch polish | Remove the required manual `ds scan` step from common onboarding workflows. |
| B | `task-freshness-sync-trust` | Immediate / patch-level | Make task sync, refresh, and stale warnings trust-preserving after manual task doc edits. |
| C | `workflow-profile-templates` | Near-term v2 | Let teams define safe local workflow profiles and task templates without arbitrary hooks. |
| D | `evidence-lanes-domain-extractors` | Mid-term v2 | Generalize domain evidence through built-in lanes and extractor contracts. |
| E | `profile-gallery-publishing-trust` | Later v2+ | Explore curated profile sharing only after local profile value is proven. |
| F | `brownfield-active-intent-ranking` | Pre-launch / early patch | Make current decision docs and exact plan IDs beat stale or tangential historical plans in find packs. |
| G | `install-self-update-utilities` | Pre-launch / early patch | Add explicit update utilities, lightweight version staleness checks, and install restart guidance. |
| H | `workspace-root-monorepo-guardrails` | Pre-launch / early patch | Detect likely workspace roots, avoid silent long scans, and make monorepo root selection understandable without full workspace support. |

## Ordering Principle

Fix local trust and workflow smoothness before adding extensibility. Profiles should come before domain extractors, and both should come before any public gallery or publishing surface.

## ScopeLab Dogfood Placement

- `A02` owns docs/onboarding polish: `ds tldr` first, two-layer PLAN/spec-to-task model, and launch docs language.
- `F` owns retrieval quality: active owner decision records, active phase docs, `Status: next` plans, and exact plan/track ID scoped packs.
- `G` owns install/update utilities: `ds update`, lightweight staleness detection, and restart shell/IDE guidance.
- `H` owns root-selection reliability: explain when DevSpecs is being run at a workspace/monorepo root, surface progress and ignored-directory behavior, and defer parallel root scanning until deterministic root grouping is proven.
- `B` remains task artifact freshness/trust. Do not overload it with package update or retrieval-ranking work.
