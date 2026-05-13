---
status: implementing
tags: [billing-webhook-hardening, entitlement_sync]
---

# Harden Entitlement Sync

Change ID: `harden-entitlement-sync`

The billing service currently derives authorization from Stripe webhooks, but webhook retries can enqueue duplicate entitlement updates. We need a narrow hardening pass that makes `stripe_event_id` idempotency durable and keeps `entitlement_sync` focused on Stripe as the billing source of truth.

## Why

Support has seen customers briefly lose access after subscription updates are replayed out of order. The durable boundary should live in the billing webhook handler, before entitlement writes fan out to auth/session materialization.

## Scope

- Add durable `webhook_replay_protection` keyed by `stripe_event_id`.
- Sync entitlement state by `customer_id` after the event is accepted once.
- Preserve `authorization_details` shape consumed by auth/session code.
- Do not implement local entitlement caching or admin override flows here.

## Non-goals

- No product packaging changes.
- No new billing dashboard UI beyond existing status display.
- No resurrecting the superseded local entitlements plan.

