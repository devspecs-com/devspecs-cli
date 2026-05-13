---
status: active
tags: [auth, session, invites]
---

# 2026-04 Team Invites Auth Rollout

This rollout plan covers workspace invitations and auth/session behavior. It is a semantic distractor for auth token queries because it mentions token refresh, session cookies, authorization details, customer access, billing permissions, and portal routes.

## Scope

Team invites let an owner invite a teammate by email. The invite flow creates an invitation token, sends an email, validates the token on accept, creates membership, and refreshes the auth session. Billing permissions update after membership changes because workspace role affects access to billing dashboard and customer portal routes.

## Boundaries

Invitation tokens are not session tokens. Invitation tokens are single-use and short-lived. Session cookies remain opaque. Billing webhook handlers do not create invitation tokens. Stripe customer IDs do not belong in invite tokens. Entitlement sync is not triggered by accepting an invite, although authorization state may reload after membership changes.

## Rollout steps

1. Add invite creation endpoint.
2. Add invite acceptance endpoint.
3. Refresh session authorization state after membership creation.
4. Verify billing dashboard access for invited admins.
5. Verify customer portal button remains owner-only.
6. Add audit log events for invite creation, resend, accept, and revoke.

## Testing matrix

Test expired invite token. Test invite accepted while session cookie is stale. Test invited member opening billing dashboard. Test invited admin lacking customer portal owner permission. Test customer portal redirect after membership change. Test auth/session reload when `authorization_details` are derived from role and entitlement state.

## Why this is not the token handoff design

The plan uses token language heavily, but it is about invitation tokens. ADR 0005 and the auth-session OpenSpec change define session token refresh. This plan should not be selected when the task asks for auth session cookie boundary design unless the query also asks about team invites.

## Why this is not billing webhook work

The plan mentions billing permissions because workspace role controls billing UI. It does not process Stripe webhooks, does not handle `stripe_event_id`, does not define replay protection, and does not update entitlement rows.

## Open questions

- Should invites inherit pending billing role changes?
- Should support be able to revoke invites from customer records?
- Should invite acceptance require another session refresh before showing billing routes?
- Should audit logs include `customer_id` when the invite grants billing-admin permissions?

