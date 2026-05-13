---
status: active
generator: cursor-plan
tags: [admin, billing, entitlement]
---

# Cursor Plan: Admin Billing Adjustments

Admin billing adjustments are a separate workstream for support-only access changes. This plan overlaps with entitlement, customer, auth, and billing language, but it should not be retrieved for Stripe webhook idempotency tasks.

## Implementation sketch

- Build an internal form that accepts `customer_id`, reason, expiry, and feature set.
- Persist adjustment metadata with actor, timestamp, and ticket URL.
- Recompute `authorization_details` after an adjustment changes.
- Show a banner in support tools when an override is active.

## Boundaries

Do not write to Stripe from the admin adjustment path. Do not process `stripe_event_id`. Do not attempt `entitlement_sync`; admin adjustments are layered after Stripe-derived entitlements. Do not alter session cookies or auth tokens.

## Retrieval note

This file is intentionally plausible for broad "billing entitlement" queries. It is only relevant when the task mentions admin overrides or support adjustments.

