# Design: Retrieval Quality Improvements

## Current State

The seed eval is deterministic and filesystem-based. It measures context retrieval/compression, not answer quality. The current retriever compresses aggressively but misses important artifacts and includes broad distractors.

The useful next step is to improve deterministic signals before adding LLM judging.

## Eval Model

### Importance-Weighted Relevance

Extend case definitions so `expected_relevant` can be either a path string or an object:

```yaml
expected_relevant:
  - path: openspec/changes/harden-entitlement-sync/design.md
    importance: must
  - path: docs/prd/billing-entitlements-v1.md
    importance: background
```

Supported importance values:

- `must`: decisive for safe agent handoff.
- `helpful`: useful but not strictly required.
- `background`: product or historical context.

Report recall by importance and overall recall.

### Context Sufficiency

Add deterministic sufficiency checks:

```yaml
success_criteria:
  must_contain_terms:
    - stripe_event_id
    - webhook_replay_protection
  must_contain_artifacts:
    - docs/adr/0002-webhook-idempotency-boundary.md
  must_not_contain_terms:
    - local entitlement cache as active plan
  must_not_contain_artifacts:
    - scratch/old-billing-plan.md
```

Sufficiency is not a proxy for LLM quality, but it checks whether the packed context contains essential terms/artifacts and avoids known misleading artifacts.

## Retrieval Signals

### Identifier-Aware Matching

Preserve and search:

- snake_case
- kebab-case
- dotted identifiers
- slash paths
- package-manager tokens such as `pnpm`
- dated filename slugs

Identifier matches should search path, title, body, extracted tasks, and source candidates.

### OpenSpec Bundle Retrieval

Represent an OpenSpec change as a linked bundle:

- `proposal.md`
- `design.md`
- `tasks.md`
- `specs/**/spec.md`

For implementation-context queries, include design/tasks/spec deltas with the proposal when they score above a threshold or when the change slug clearly matches. For narrow identifier queries, allow a smaller subset.

### Authority and Lifecycle

Introduce general authority/status signals:

- Accepted ADRs are high-authority for decisions.
- Superseded ADRs are high-authority only for stale/history queries.
- Active OpenSpec changes are high-authority for implementation context.
- PRDs are useful for product-background queries and lower priority for implementation-only queries.
- Scratch files and stale notes are low authority unless the query asks for stale, old, history, or superseded context.
- Cursor/Claude notes are useful for resume/follow-up but should not outrank authoritative ADR/OpenSpec design by default.

### Query Intent

Classify query intent deterministically:

- implementation context
- source/code identifier
- product/background
- decision/rationale
- stale/history
- resume/continue

Use intent to adjust artifact-type preference in broad ways. Avoid file-specific boosts.

### Explainability

For each included artifact, capture score reasons:

- matched terms
- path/title/body/source match
- identifier match
- authority/status signal
- OpenSpec bundle inclusion
- query intent influence

Expose reasons in eval JSON first. Human output can remain compact.

## Risk

The highest risk is overfitting the seed fixture. Mitigation:

- Treat `agentic-saas-fragmented-v1` as `seed_smoke`.
- Keep visible misses and noisy inclusions in output.
- Prefer trial-report-derived fixes.
- Add held-out or locked fixtures before marketing claims.

