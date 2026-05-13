---
status: accepted
tags: [billing-webhook-hardening, webhook_replay_protection]
---

# ADR 0002: Webhook Idempotency Boundary

## Decision

The durable idempotency boundary for billing webhooks is the API webhook handler, keyed by `stripe_event_id`.

## Context

Stripe retries events and can deliver them out of order. If duplicate delivery reaches entitlement writers, `entitlement_sync` can create confusing intermediate access state for a `customer_id`.

## Consequences

- The handler records `stripe_event_id` before entitlement side effects.
- Duplicate event inserts are treated as successful `webhook_replay_protection` no-ops.
- The entitlement sync layer remains responsible for loading current Stripe state and writing `authorization_details`.
- Local caches are not the consistency boundary.

