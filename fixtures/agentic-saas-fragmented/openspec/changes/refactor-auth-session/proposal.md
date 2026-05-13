---
status: implementing
tags: [auth, session, token]
---

# Refactor Auth Session Token Handoff

Change ID: `refactor-auth-session`

The auth service should own token refresh and session cookie validation. The web app should not shuttle short-lived auth tokens through billing or customer portal routes. This proposal is intentionally adjacent to billing because `authorization_details` flow through the session boundary.

## Scope

- Move token refresh behind the API session boundary.
- Keep `authorization_details` loaded after session validation.
- Ensure billing dashboard and customer portal redirects use the same session guard.
- Add spec coverage for rejected browser-visible token handoff behavior.

## Non-goals

- No Stripe webhook idempotency changes.
- No `stripe_event_id` handling.
- No entitlement sync changes beyond reading the authorization materialized state.

