---
status: active
source: claude-notes
tags: [entitlement, rollout, retrospective]
---

# Claude Notes: Entitlement Rollout Retrospective

This retrospective summarizes a rollout where entitlement materialization, auth/session loading, customer portal redirects, and Stripe webhook replay all interacted. It is valuable for broad historical context and noisy for targeted implementation retrieval.

## What happened

The team rolled out entitlement materialization behind a feature flag. Most customers converged quickly, but a few workspaces saw access flicker after subscription changes. Investigation mentioned `customer_id`, `authorization_details`, `stripe_event_id`, webhook replay, auth token refresh, and local entitlement caching.

The final conclusion was that multiple issues overlapped. Some customers had duplicate Stripe customer records. Some portal redirects returned before entitlement materialization finished. Some sessions loaded authorization state before the materializer wrote new rows. Some webhook replays were harmless but looked alarming in logs.

## Lessons

- Do not treat every billing incident as a webhook bug.
- Do not treat every auth/session incident as an entitlement bug.
- Durable idempotency belongs at the webhook boundary.
- Product dashboards need pending states when entitlement convergence lags.
- Support needs a single runbook that separates Stripe state, entitlement state, auth/session state, and admin overrides.

## Why this note exists

This note helps humans understand the messy history. It should not be the primary context for code changes. It deliberately shares vocabulary with most billing/auth eval cases so retrieval precision can be measured against realistic distractors.

