# Design: Billing Webhook Hardening

This design backs `harden-entitlement-sync` and the `billing-webhook-hardening` workstream.

## Boundary

The idempotency boundary is the API webhook handler. It must attempt an insert into `billing_webhook_events` using `stripe_event_id` before calling entitlement write code. A duplicate insert means the webhook is a replay and the handler should return a 2xx response without running `entitlement_sync` again.

## Data model

`billing_webhook_events` stores:

- `stripe_event_id` as the primary key.
- `customer_id` copied from the Stripe object.
- `event_type` for support debugging.
- `processed_at` for replay analysis.

## Entitlement sync

Accepted subscription events call the entitlement sync path with `customer_id`. The sync path reloads Stripe subscription state, writes the normalized entitlement row, and updates `authorization_details` consumed by auth/session.

## Replay behavior

`webhook_replay_protection` treats replayed events as successful no-ops. It should not emit user-visible errors, retry jobs, or new entitlement revisions for the same `stripe_event_id`.

## Open questions

- Whether to record ignored event types in the same table.
- Whether the entitlement write should be wrapped in the same transaction as the event insert.

