---
status: active
tags: [onboarding, activation, workspace]
---

# 2026-04 Onboarding Activation Playbook

This playbook covers workspace onboarding and activation. It is mostly unrelated to billing webhook hardening, but realistic SaaS repos often keep onboarding, billing, auth, support, and customer plans together. The file adds corpus weight and mild semantic noise without being expected in any current eval case.

## Activation funnel

The onboarding funnel starts when a user creates a workspace, invites teammates, connects a first integration, and completes a first project. Billing appears only when the workspace exits trial or opens the customer portal. Auth/session appears because onboarding screens depend on user identity and workspace role. Entitlement appears because paid features are hidden until the workspace has the right plan.

## Milestones

1. Workspace created.
2. Owner verifies email.
3. Owner invites teammate.
4. Teammate accepts invite.
5. Workspace connects an integration.
6. Workspace creates first project.
7. Workspace reaches trial limit.
8. Owner opens billing dashboard or checkout.
9. Workspace converts or enters lifecycle nurture.

## Product notes

The onboarding system should not know about `stripe_event_id`. It should not implement webhook replay protection. It should not refresh auth tokens outside the session layer. It may read `authorization_details` to determine which activation prompts are relevant, but it must treat that data as derived authorization state.

## Support notes

Support often sees onboarding tickets that mention billing, customer, token, session, and entitlement. For example, a user may say "I invited a customer and billing says my token expired." Usually that means the user mixed up a workspace invite token, auth session expiry, and billing dashboard permissions. The correct context is onboarding or auth, not Stripe webhook processing.

## Instrumentation

Activation events should include workspace ID, actor ID, event name, and timestamp. They should not include raw auth token values. They should not include raw `authorization_details`. They should not include Stripe payloads. Billing conversion events may reference a `customer_id`, but only after the billing service creates one.

## Repeated implementation cautions

- Keep onboarding state separate from entitlement state.
- Keep invite tokens separate from session tokens.
- Keep billing dashboard prompts separate from customer portal session creation.
- Keep support notes out of source-of-truth planning when a narrower ADR or OpenSpec change exists.
- Keep product activation metrics separate from webhook replay metrics.

## Rollout checklist

- Add activation event taxonomy.
- Add first-project empty state.
- Add invite nudges.
- Add trial limit education.
- Add owner-only billing prompt.
- Add support copy for expired invite tokens.
- Add analytics dashboard for activation milestones.
- Add regression tests for session reload after invite acceptance.

## Long-form rationale

Onboarding is broad, and broad docs are dangerous for retrieval. They collect terms from every adjacent system because onboarding is where customers meet the product. A compact context packer should avoid including this playbook for narrow billing or auth implementation tasks. It is useful for product planning, not for webhook idempotency, OpenSpec spec deltas, or source-file-level agent work.

