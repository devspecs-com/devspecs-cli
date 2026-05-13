# Design: Auth Session Cookie Boundary

The design follows ADR 0005. The core idea is to keep session token refresh server-side and make auth/session the boundary where identity, workspace membership, and `authorization_details` are loaded.

## Current problem

The old token handoff path allowed the web app to carry a short-lived token through billing dashboard transitions. Customer portal redirects could race with token refresh and produce confusing failures that looked like billing entitlement bugs.

## New flow

1. Browser sends the opaque session cookie.
2. API session middleware validates the cookie.
3. Auth infrastructure refreshes the internal session token if needed.
4. Session code loads workspace membership and entitlement-derived `authorization_details`.
5. Billing and portal handlers receive a validated session object, not a browser-visible token.

## Rationale

This keeps auth token behavior out of billing webhook code. Webhooks may update entitlement rows by `customer_id`, but they should not invalidate cookies or issue tokens. The design also gives agents a clean answer when a query mentions auth token, session, customer portal, billing dashboard, and authorization details at the same time.

