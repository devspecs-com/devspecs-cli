---
status: active
tags: [entitlement_sync, billing-webhook-hardening]
---

# 2026-05-01 Entitlement Sync Plan

Plan the hardening sequence for `harden-entitlement-sync`:

1. Land the `stripe_event_id` migration.
2. Add durable webhook replay checks.
3. Make `entitlement_sync` reload by `customer_id`.
4. Refresh `authorization_details` for auth/session.

The plan predates ADR 0002, so use the ADR for the final idempotency boundary.

