---
status: accepted
tags: [auth, session, token]
---

# ADR 0005: Auth Session Cookie Boundary

## Decision

The API owns the session cookie boundary. Browser clients receive an opaque session cookie, not a long-lived bearer token, and auth/session code is responsible for loading `authorization_details` after the session is validated.

## Context

The previous auth token handoff passed a short-lived token through the web app and expected the billing dashboard to refresh it during route transitions. That made token refresh behavior depend on React rendering and confused billing failures with auth failures.

The new boundary keeps token refresh server-side. The session layer loads user identity, workspace membership, and entitlement-derived `authorization_details`. Billing webhooks may update entitlement rows, but they do not write browser cookies or issue auth tokens.

## Consequences

- `services/api/src/auth/session.ts` validates the cookie and loads current authorization state.
- Token refresh code belongs in auth infrastructure, not billing webhook handling.
- Webhook replay protection must not invalidate sessions.
- Customer portal redirects must re-check session state before creating Stripe portal sessions.
- Any future token handoff design must include an OpenSpec spec delta for auth behavior.

## Retrieval warning

Queries containing "auth token", "session", "cookie", or "authorization_details" should prefer this ADR and auth source files over billing webhook docs unless the query explicitly asks for webhook implementation.

