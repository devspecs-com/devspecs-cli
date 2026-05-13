---
status: active
tags: [billing, invoices, export]
---

# 2026-04 Invoice Export Workflow

This plan covers invoice export and finance reporting. It lives near billing artifacts and shares customer, Stripe, portal, auth, session, and token vocabulary, but it is unrelated to entitlement sync or webhook idempotency implementation.

## Problem

Finance admins need a predictable way to export invoices for a workspace. Today they ask support for invoice PDFs or use the Stripe customer portal directly. The product goal is to make invoice export visible inside the billing dashboard without exposing internal subscription records or raw webhook payloads.

## Scope

- Add invoice list and export actions to the billing dashboard.
- Read invoice metadata from Stripe using the workspace `customer_id`.
- Require a valid auth session before exporting invoice links.
- Record support audit events when staff export on behalf of customers.
- Keep portal session creation separate from invoice export.

## Non-goals

- No entitlement sync changes.
- No `stripe_event_id` idempotency changes.
- No webhook replay behavior changes.
- No browser-visible auth token storage.
- No admin billing override changes.

## Workflow details

The API should load the current workspace session, verify billing permissions, read the canonical `customer_id`, and request invoice metadata from Stripe. The response should contain invoice IDs, dates, amounts, and hosted invoice URLs. It should not include payment method details. It should not include `authorization_details` except insofar as the session guard uses them to determine access.

Invoice exports may be requested after a customer portal session changes billing address or tax ID. That does not make this workflow responsible for customer portal implementation. The portal endpoint creates a hosted portal session; invoice export reads invoice data. Both use `customer_id`, both require auth/session validation, and both appear in billing support conversations.

## Failure modes

If Stripe is unavailable, the dashboard should show a retry state. If the user lacks billing permissions, the API should return authorization failure. If the workspace has no `customer_id`, the dashboard should suggest checkout or support contact. If invoice metadata does not match the current workspace, support should investigate customer migration notes rather than webhook replay.

## Search ambiguity

This document mentions replay, idempotency, tokens, sessions, authorization, customer portal, Stripe, billing, and support. Those mentions are operational context. They should not make the document relevant to agent tasks about implementing durable webhook replay protection, auth token handoff design, or local entitlement cache history.

## Rollout

The rollout is low risk because invoice export is read-only. Product wants the feature behind an owner-only permission check for the first release. Support wants logging that includes user ID, workspace ID, `customer_id`, invoice ID, and export timestamp. Engineering wants no coupling to entitlement materialization so invoice export can continue even when authorization details are stale but the session itself is valid.

## Acceptance notes

- Owner can list invoices.
- Owner can open hosted invoice URL.
- Non-owner cannot export invoice data.
- API does not expose raw Stripe payloads.
- Export does not mutate billing, entitlement, webhook, token, or session state.

