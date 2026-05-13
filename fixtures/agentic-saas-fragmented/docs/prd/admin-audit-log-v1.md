---
status: draft
kind: requirements
subtype: prd
tags: [admin, audit, support]
---

# PRD: Admin Audit Log v1

Admin Audit Log v1 records support and admin actions that affect customer accounts, billing settings, team membership, auth/session state, and temporary entitlement overrides. It is relevant background for support tooling and a distractor for billing/auth implementation searches.

## Goals

- Record who changed a customer record.
- Record who created a customer portal session on behalf of a customer.
- Record who applied an admin billing override.
- Record who revoked an invitation token.
- Record when support viewed invoice exports.
- Make audit entries searchable by workspace, user, `customer_id`, and action type.

## Non-goals

- Do not implement Stripe webhook idempotency.
- Do not process webhook replay.
- Do not refresh auth tokens.
- Do not write entitlement rows.
- Do not expose raw `authorization_details`.

## Requirements

Audit events must include actor, subject, workspace, timestamp, and reason. Billing-related events may include `customer_id`. Auth-related events may include session identifier hash but not the raw token. Entitlement-related events may include override metadata but not full authorization details. Webhook-related events may include event type and replay flag but not the raw Stripe payload.

## Product notes

Support wants a single timeline for customer issues. Engineering wants the timeline to clarify which subsystem changed state: billing, auth/session, entitlement materialization, admin override, invite flow, customer portal, or invoice export. Product wants the timeline to reduce escalations caused by ambiguous terms like token, replay, customer, and authorization.

## Retrieval note

This PRD has broad vocabulary overlap with almost every fixture case. It should be relevant for audit-log product work. It should not be relevant for narrow implementation tasks about webhook replay protection, auth session token handoff, pnpm migration, or local entitlement caching history.

