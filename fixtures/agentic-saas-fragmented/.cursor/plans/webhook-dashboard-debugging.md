---
status: active
generator: cursor-plan
tags: [webhook, dashboard, stripe]
---

# Cursor Plan: Webhook Dashboard Debugging

This plan is about debugging a support dashboard for Stripe webhook delivery. It shares the words webhook, replay, idempotency, customer, token, and billing, but its implementation target is observability, not entitlement writes.

## Proposed work

- Add a dashboard table for recent Stripe webhook deliveries.
- Filter by `customer_id`, event type, retry count, and status.
- Link dashboard rows to support cases.
- Highlight replayed deliveries so support can see why an event did not create a new entitlement revision.
- Avoid exposing raw `authorization_details` in the dashboard.

## Why this is noisy

Support asked whether duplicate webhooks were causing access flicker. The dashboard can help answer that question, but the dashboard is downstream of `webhook_replay_protection`. Agents implementing durable replay protection should not start here unless the task is explicitly about observability.

## Notes

Use read-only queries against webhook event logs. Do not change `stripe_event_id` insert behavior. Do not update the entitlement sync path. Do not adjust auth/session token handling.

