---
status: accepted
---

# ADR 0001: Use Stripe as Billing Source

Stripe remains the authoritative billing system for subscription state. DevSpecs should treat `customer_id` as the join key between Stripe and internal entitlement records.

## Consequences

- `entitlement_sync` should reload state from Stripe instead of trusting webhook payload snapshots.
- `authorization_details` in auth/session should be derived from normalized entitlement rows.
- Product reporting can lag behind webhook processing as long as access checks converge quickly.

