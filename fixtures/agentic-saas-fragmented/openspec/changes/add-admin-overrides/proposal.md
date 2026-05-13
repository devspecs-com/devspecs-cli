---
status: proposed
tags: [admin, authorization_details]
---

# Add Admin Entitlement Overrides

This unrelated proposal lets support staff grant temporary feature access to a `customer_id` during enterprise pilots.

The work intentionally happens after `harden-entitlement-sync` so billing remains the source of truth. It may extend `authorization_details` with an override reason, but it does not change `stripe_event_id`, `entitlement_sync`, or webhook replay handling.

