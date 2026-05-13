# Customer Portal Chaos Notes

Status: stale scratch.

This scratch note collected messy observations from early customer portal testing. It mentions billing, webhook replay, Stripe, entitlement sync, auth token refresh, session cookies, `customer_id`, `authorization_details`, admin overrides, and invoice status. It is intentionally too broad for most focused tasks.

## Observations

Some portal sessions returned while webhooks were still in flight. Some users refreshed the billing dashboard before entitlement materialization completed. Some auth sessions had stale authorization state. Some support overrides expired during testing. Some duplicate Stripe customers confused invoice lookups.

The note proposed several rejected ideas:

- Have customer portal endpoint trigger `entitlement_sync`.
- Store portal state in auth token claims.
- Invalidate sessions from webhook handlers.
- Treat replayed webhooks as user-visible errors.
- Let support edit local entitlement cache rows.

All of those ideas were rejected by later ADRs and OpenSpec designs. This note is a useful precision distractor because it has many term matches and little authoritative value.

## Search noise inventory

Search terms likely to hit this file include webhook, replay, idempotency, Stripe, customer, portal, token, session, auth, billing, entitlement, `stripe_event_id`, `customer_id`, and `authorization_details`. A good context packer should only include it when the user explicitly asks for stale scratch history or rejected ideas.

