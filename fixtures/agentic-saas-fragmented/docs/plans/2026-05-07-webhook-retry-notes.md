---
status: active
tags: [webhook_replay_protection, stripe_event_id]
---

# 2026-05-07 Webhook Retry Notes

Stripe replay tests showed duplicate `customer.subscription.updated` events with the same `stripe_event_id`. The handler should return success for duplicate event IDs after the insert-once check.

Follow-up: verify that replayed events do not trigger `entitlement_sync` or mutate `authorization_details`.

