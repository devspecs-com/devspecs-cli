---
status: draft
kind: requirements
subtype: prd
tags: [billing, customer-portal, stripe]
---

# PRD: Customer Portal v2

Customer Portal v2 gives workspace owners a self-serve path for payment method updates, invoice review, and seat-count changes. It is product background for portal work, not implementation context for webhook replay protection.

## Goals

- Reduce support tickets for billing address and payment method changes.
- Let workspace owners open Stripe-hosted portal sessions from the billing dashboard.
- Make invoice download status visible without exposing internal subscription objects.
- Use the current `customer_id` and workspace session to scope portal access.

## Non-goals

- Do not replace `entitlement_sync`.
- Do not change `stripe_event_id` handling.
- Do not write `authorization_details` from the portal endpoint.
- Do not implement admin billing overrides.

## User stories

As a workspace owner, I can open the portal from the billing dashboard and return to the app after making changes. As a finance admin, I can confirm that invoices belong to the current `customer_id`. As support, I can distinguish portal access errors from webhook replay processing.

## Ambiguous terms

This PRD deliberately repeats billing, Stripe, customer, auth session, token, webhook, replay, and entitlement language because real product docs do. Retrieval should not include it for narrow implementation queries such as "implement webhook replay protection", but it is relevant when the task asks for product background or portal UX.

