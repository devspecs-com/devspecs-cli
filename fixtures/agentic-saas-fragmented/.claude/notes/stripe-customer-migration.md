---
status: active
source: claude-notes
tags: [stripe, customer, migration]
---

# Claude Notes: Stripe Customer Migration

The migration thread tracks cleanup of duplicate Stripe customers and old checkout sessions. It contains many terms that collide with entitlement sync: `customer_id`, Stripe, billing, auth, session, webhook, replay, and token.

## Important distinction

The migration does not change how webhook events are processed. It does not implement idempotency, does not define `webhook_replay_protection`, and does not update `authorization_details`. Its only durable output is a cleaner mapping from workspace to Stripe `customer_id`.

## Follow-up list

- Confirm migrated customers can still open customer portal sessions.
- Verify support dashboards no longer display obsolete customer aliases.
- Keep old customer records read-only until invoice exports are reconciled.
- Avoid touching entitlement sync code unless a migrated customer lacks a subscription row.

