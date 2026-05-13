---
status: proposed
tags: [billing, customer-portal, stripe]
---

# Add Customer Portal

This OpenSpec change adds Stripe-hosted customer portal access from the billing dashboard. It shares Stripe, webhook, customer, token, auth, and entitlement vocabulary, but it is not the active `harden-entitlement-sync` work.

## Scope

- Add a portal session endpoint that accepts the current workspace `customer_id`.
- Add billing dashboard entry points for workspace owners.
- Return users to the billing dashboard after Stripe-hosted portal changes.
- Keep subscription changes flowing through webhook processing and entitlement sync.

## Non-goals

- No `stripe_event_id` idempotency changes.
- No `webhook_replay_protection` implementation.
- No admin billing override work.

