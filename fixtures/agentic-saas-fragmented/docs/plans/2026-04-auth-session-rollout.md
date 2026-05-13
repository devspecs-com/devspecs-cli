---
status: active
tags: [auth, session, token, rollout]
---

# 2026-04 Auth Session Rollout Plan

This rollout plan tracks operational work for moving token refresh behind the API session boundary. It shares language with ADR 0005 and the `refactor-auth-session` OpenSpec change, but it is less authoritative than those artifacts.

## Rollout stages

Stage one keeps the existing browser-visible token handoff in place while adding observability around session cookie validation. Stage two adds server-side token refresh behind the API session boundary. Stage three removes billing dashboard token refresh hooks. Stage four validates customer portal redirects and entitlement-derived `authorization_details` after login.

## Validation checklist

- Confirm API session middleware can refresh internal tokens without involving React route transitions.
- Confirm billing dashboard requests do not receive browser-visible bearer tokens.
- Confirm customer portal return URLs re-check session state.
- Confirm `authorization_details` load after entitlement materialization.
- Confirm Stripe webhook replay does not invalidate a user session.
- Confirm support can distinguish auth token expiry from billing entitlement drift.

## Ambiguous terminology

The rollout mentions billing, customer, portal, webhook, entitlement, and authorization because auth touches all product surfaces. It should not be retrieved for `stripe_event_id` idempotency unless the query explicitly asks about session behavior during billing changes.

## Risk register

Risk: users with long-lived browser tabs may keep stale session state. Mitigation: server-side session validation should refresh internal token state before loading authorization details.

Risk: customer portal redirects may return to a route that assumes billing state has already converged. Mitigation: dashboard should reload after redirect and show pending state if entitlement materialization lags.

Risk: support may confuse auth token failures with billing failures. Mitigation: logging should attach auth/session outcome, customer ID, and entitlement materialization version to support traces.

Risk: old scratch plans may suggest storing `customer_id` in a browser token. Mitigation: ADR 0005 explicitly rejects that design.

## Implementation notes

This plan intentionally avoids code-level instructions. The authoritative design is in the OpenSpec change and ADR. Agents implementing the session boundary should read those first. Agents implementing webhook replay protection should not use this rollout plan as primary context.

## Drill notes

During internal testing, a portal redirect returned after the session token expired. The new boundary refreshed the token server-side, loaded `authorization_details`, and then allowed the billing dashboard to render. This verified the auth/session handoff but did not exercise Stripe webhook idempotency.

During a separate test, a webhook replay arrived while a user refreshed the dashboard. The session layer behaved correctly. The duplicate event handling still belonged to billing webhook code. This distinction is important for retrieval precision because the same words appear in both threads.

