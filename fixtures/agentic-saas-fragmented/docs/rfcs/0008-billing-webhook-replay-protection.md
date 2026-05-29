---
status: accepted
tags: [billing, webhook, replay, rfc]
---

# RFC 0008: Billing Webhook Replay Protection

## Summary

Use `stripe_event_id` as the durable replay ledger key for billing webhook handling and keep customer portal session creation out of the idempotency boundary.

## Motivation

Webhook replay protection has implementation details in OpenSpec and source code, but the RFC records the design options and why event-level idempotency was selected. It should be useful when an agent asks for proposal or alternatives context.

## Goals

- Treat repeated Stripe delivery attempts as no-op replays.
- Preserve the `webhook_replay_protection` invariant across retries.
- Keep entitlement sync driven by Stripe events and `customer_id`.

## Non-Goals

- Do not solve customer portal session idempotency.
- Do not replace the entitlement sync OpenSpec change.

## Proposal

Persist every processed `stripe_event_id` before running entitlement mutation. If a later delivery has the same event ID, return success without reapplying entitlement changes.

## Detailed Design

The webhook handler in `services/api/src/billing/webhooks.ts` checks the replay ledger before calling entitlement sync. The migration `20260501090000_add_stripe_event_id.sql` adds the storage column and uniqueness boundary.

## Drawbacks

The ledger adds a write on every billing webhook.

## Alternatives

- Use `customer_id` as the replay key.
- Rely only on Stripe delivery ordering.
- Keep replay detection in support dashboards rather than the handler.
