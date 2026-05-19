---
status: accepted
tags: [auth, session, token, rfc]
---

# RFC 0007: Auth Session Token Handoff

## Summary

Move session cookie validation ahead of token claim enrichment so `authorization_details` are read from the server session boundary and never from browser-visible token fields.

## Motivation

The auth refactor needs a stable handoff between cookie validation, server session loading, and downstream billing access checks. A proposal-shaped record is useful here because several implementation plans discuss the rollout while the RFC captures the design tradeoff.

## Proposal

`services/api/src/auth/session.ts` owns the session cookie boundary. `services/api/src/auth/tokens.ts` may read a stable user ID and token metadata, but it must not mint or persist `authorization_details`.

## Detailed Design

- Validate the session cookie before token enrichment.
- Load `customer_id` and `authorization_details` from the server session record.
- Keep token refresh code unaware of billing entitlement materialization.

## Drawbacks

The handoff adds one extra server lookup during auth-sensitive requests.

## Alternatives

- Store billing claims directly in browser-visible tokens.
- Reload billing entitlements from every handler.

## Open Questions

- Should support tooling display missing `authorization_details` as an auth issue or a billing materialization issue?
