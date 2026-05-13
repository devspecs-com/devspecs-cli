---
status: stale
source: claude-notes
tags: [entitlements, local-cache, experiment]
---

# Claude Notes: Local Entitlements Experiment

This experiment explored local entitlement caching before ADR 0001 and ADR 0002 were accepted. It is useful historical context for "why not local entitlements" questions and a distractor for active entitlement sync implementation.

## Experiment summary

The idea was to cache `authorization_details` locally and refresh them with a scheduled job. Webhooks would only mark a `customer_id` dirty, and a worker would later reconcile Stripe subscription state. The experiment appeared attractive because auth/session reads would be fast and billing webhook handlers would do less work.

## Why it failed

- It did not define a durable `stripe_event_id` idempotency boundary.
- Replay behavior was hard to reason about because duplicated events only marked cache rows dirty.
- Support could not tell whether a stale token reflected billing state, cache lag, or auth/session failure.
- Product wanted faster convergence after Stripe plan changes.

## Current guidance

Use this note only when the query asks about stale local entitlement caching, superseded plans, or historical rationale. Do not use it for active `harden-entitlement-sync` implementation.

