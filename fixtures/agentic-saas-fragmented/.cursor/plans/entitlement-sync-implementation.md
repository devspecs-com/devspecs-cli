---
status: active
generator: cursor-plan
tags: [entitlement_sync, billing-webhook-hardening]
---

# Cursor Plan: Entitlement Sync Implementation

Implementation order:

- Start in `services/api/src/billing/webhooks.ts`.
- Introduce a repository helper that inserts `stripe_event_id` and returns a replay flag.
- Call `entitlement_sync` only after insert succeeds.
- Keep the `customer_id` extraction close to the webhook event parser.
- Leave admin override `authorization_details` changes for the separate OpenSpec proposal.

