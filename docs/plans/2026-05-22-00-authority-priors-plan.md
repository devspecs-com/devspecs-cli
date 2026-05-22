# Authority Priors Plan

Date: 2026-05-22

Order: 0

Depends on: current file-level retrieval eval.

Enables:

- [1] Duplicate / variant collapse
- [2] Section indexing + packing
- [3] Tiered output
- [4] Query intent routing / lane budgets

## Goal

Add explainable authority priors to retrieval ranking so high-trust artifacts are easier to select without turning authority into a hard filter.

Authority priors should help answer:

- Is this artifact canonical or a copy/example/archive?
- Is this artifact a recognized intent/protocol/model/template surface?
- Is this artifact current enough to trust?
- Is this artifact in a configured source path?
- If git is available, is this file stable or churn-heavy?

## Design

Introduce an `AuthorityPrior` score alongside lexical/query score.

Suggested signals:

- Configured source path match.
- Adapter/classifier confidence.
- Artifact subtype and lane.
- Canonical path shape, such as `docs/adr`, `docs/rfcs`, `docs/product-specs`, `openspec/specs`, `openspec/changes`.
- Lifecycle status, such as accepted, active, draft, superseded, archived, stale.
- OpenSpec role and parent/child structure.
- Path negative hints, such as archive, generated, example, template, legacy, translated mirror.
- Optional git stability signal when `.git` is available:
  - observed change count for the file
  - last modified commit age
  - whether file moved/renamed often
  - never penalize missing git data

Authority priors should be additive or subtractive ranking features only. They must not exclude candidates by themselves.

## Git Signal Policy

Git-derived authority is optional and conservative.

- Missing git data means neutral.
- High churn should not be an automatic penalty; it may mean the file is active and current.
- Low churn plus old accepted status can boost durable decisions.
- Recent changes can boost active plans/specs.
- Git features must be disabled or cheap when the repo is not a git worktree.

## Eval Integration Lift

Low.

The current file-level eval can measure impact directly:

- precision
- recall
- must-have recall
- context sufficiency

Add only diagnostics at first:

- per-artifact authority score
- authority reasons
- whether git was available

No new labels are required.

## Auditable Success Criteria

- No candidate is excluded solely by an authority prior.
- Canonical fixture must-have recall does not regress.
- Canonical classifier fixture remains at current pass count.
- Real dev50 must-have recall stays at or above the current optimized baseline.
- Real dev50 precision improves or stays neutral without reducing context sufficiency.
- Retrieval reasons expose the top authority prior factors for every included artifact.
- Unit tests cover neutral behavior when git is absent.
- Unit tests cover stable/durable accepted docs and active/recent plans without repo-specific literals.

## Rollback Criteria

- Any must-have recall drop that cannot be explained by label noise.
- Authority score hides or demotes exact path/title matches in favor of generic canonical docs.
- Git probing noticeably slows scan/retrieval in non-git or large-repo cases.
