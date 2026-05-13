---
status: draft
kind: requirements
subtype: prd
tags: [billing, analytics, support]
---

# PRD: Billing Analytics v1

Billing Analytics v1 gives internal teams a way to understand subscription movement, customer portal usage, webhook replay volume, entitlement convergence, and support interventions. It is product background for analytics and operations, not implementation context for entitlement sync.

## Goals

- Show weekly active paid customers by plan.
- Show customer portal session counts and conversion after invoice failures.
- Show webhook replay volume grouped by event type.
- Show entitlement materialization latency after subscription changes.
- Show auth/session failures that block access to billing surfaces.
- Show admin override volume by support team and expiry date.

## Non-goals

- Do not alter `webhook_replay_protection`.
- Do not implement `stripe_event_id` idempotency.
- Do not change auth token refresh.
- Do not write `authorization_details`.
- Do not redefine `customer_id` ownership.

## User stories

As support, I can see whether a customer's billing issue is caused by Stripe, portal access, webhook processing, entitlement lag, auth/session state, or an admin override. As product, I can understand how many customers use portal self-service instead of contacting support. As engineering, I can watch replay volume without using analytics dashboards as source of truth.

## Retrieval warning

This PRD is intentionally packed with terms that appear in implementation tasks. It should be relevant to product-background queries. It should be irrelevant to narrow agent-context queries that ask for source files, OpenSpec design, ADR boundaries, or specific identifiers.

## Requirements

Analytics events must never contain raw session tokens. Portal events may contain `customer_id` but not payment details. Webhook events may contain `stripe_event_id` but should not expose raw webhook payloads to product dashboards. Entitlement convergence metrics may include counts and latency but not full `authorization_details`.

## Future ideas

The team may later add anomaly detection for replay storms, portal failures, and stale authorization state. Those ideas are useful for roadmap planning but should not be confused with the active billing webhook hardening implementation.

