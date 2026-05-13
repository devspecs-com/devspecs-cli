---
status: accepted
kind: requirements
subtype: prd
tags: [billing, entitlements]
---

# PRD: Billing Entitlements v1

The product goal is to ensure paid customers keep access to the features they bought and trial customers see clear upgrade paths.

## User outcomes

- Workspace owners can see plan, seats, and billing state.
- Auth checks use `authorization_details` that reflect the current subscription.
- Support can diagnose access problems by `customer_id`.

## Engineering notes

The PRD mentions `entitlement_sync` as the implementation mechanism, but it does not define `stripe_event_id` idempotency details. Those belong in ADR 0002 and the active OpenSpec change.

