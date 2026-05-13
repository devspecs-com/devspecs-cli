---
status: accepted
tags: [webhook, observability, support]
---

# ADR 0006: Webhook Observability Boundary

## Decision

Webhook observability is read-only support tooling. The dashboard may show Stripe event type, `stripe_event_id`, replay flag, `customer_id`, processing status, and entitlement sync outcome, but it must not become the source of truth for replay protection.

## Context

Support needs to understand why a customer saw a billing or access issue. The dashboard sits near webhook idempotency, entitlement sync, auth/session reloads, and customer portal changes. That proximity creates retrieval ambiguity. The dashboard can explain a replay. It does not define what a replay means.

## Consequences

- Dashboard queries read from webhook event records.
- Dashboard code does not insert `stripe_event_id`.
- Dashboard code does not call `entitlement_sync`.
- Dashboard code does not write `authorization_details`.
- Dashboard code does not refresh auth tokens or session cookies.

## Relation to ADR 0002

ADR 0002 defines the durable idempotency boundary. This ADR defines observability around that boundary. Agent context for implementing replay protection should include ADR 0002 before this ADR. Agent context for building support dashboards may include both.

