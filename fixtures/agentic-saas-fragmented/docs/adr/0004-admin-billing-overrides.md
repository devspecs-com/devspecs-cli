---
status: accepted
tags: [admin, billing, authorization_details]
---

# ADR 0004: Admin Billing Overrides

## Decision

Support may apply time-bounded billing overrides for enterprise pilots, but overrides must be separate from Stripe-derived entitlement state.

## Context

The support team occasionally needs to grant temporary access while procurement finishes. This overlaps with `authorization_details`, auth/session checks, customer records, and billing UX. It should not be confused with webhook idempotency or the Stripe source-of-truth decision.

## Consequences

- Override state is stored with an expiry timestamp and an audit actor.
- `authorization_details` may include an override reason after the entitlement materializer combines Stripe state with override state.
- The `customer_id` remains the billing join key.
- Admin overrides do not process `stripe_event_id`.
- Admin override writes do not run `entitlement_sync`; they trigger a separate authorization materialization step.

## Non-goals

This ADR does not define replay protection, portal UX, auth cookie boundaries, or local entitlement caching. It intentionally competes for terms like billing, entitlement, authorization, and customer, making it a useful distractor for retrieval precision.

