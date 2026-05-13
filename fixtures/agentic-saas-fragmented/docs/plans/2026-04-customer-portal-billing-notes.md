---
status: active
tags: [billing, customer-portal, stripe]
---

# 2026-04 Customer Portal Billing Notes

The customer portal project lets workspace owners update payment methods, download invoices, and open Stripe-hosted billing sessions. It is related to billing, Stripe, and `customer_id`, but it is intentionally outside the `harden-entitlement-sync` implementation path.

## Working assumptions

- Portal sessions are created from the web app after auth/session validation.
- The API passes the current `customer_id` to Stripe and receives a hosted portal URL.
- The portal must not write `authorization_details` directly.
- Subscription changes still arrive through Stripe webhooks and are processed by the entitlement sync path.
- Portal errors are user-facing; webhook replay errors are operational and should be silent no-ops when duplicates are detected.

## Details that create retrieval ambiguity

The portal flow mentions webhooks because users may change plans inside Stripe. It mentions idempotency because portal session creation should not create duplicate sessions on double-click. It mentions replay because browser retries can repeat the request. These are not the same as `webhook_replay_protection` in the API webhook handler.

The plan also mentions `entitlement_sync` only to say that portal changes must not bypass it. If a task asks for billing portal UX or invoice access, this plan is relevant. If the task asks to implement durable Stripe webhook idempotency, this plan is a distractor.

## Checklist

- Add "Manage billing" button in the billing dashboard.
- Ensure portal session creation requires an active workspace session.
- Use `customer_id` from the workspace billing record, not user profile metadata.
- Redirect back to `/billing` after the Stripe-hosted session.
- Record portal session creation in audit logs for support.
- Do not update entitlements from the portal endpoint.
- Do not handle `stripe_event_id` here.

## Open questions

- Should trial workspaces without a `customer_id` see checkout instead of portal?
- Should invoice download links be mirrored into the app?
- How should support distinguish portal errors from webhook processing errors?

