---
status: active
tags: [security, retention, audit]
---

# 2026-04 Data Retention Security Review

This security review covers retention for operational records across billing, auth, support, notifications, audit logs, and customer portal flows. It is intentionally broad. The review mentions many terms that appear in focused implementation work, but it is not itself implementation context for any current eval case.

## Review goals

- Define retention windows for billing support logs.
- Define retention windows for auth/session audit traces.
- Define retention windows for customer portal session audit entries.
- Define retention windows for notification delivery logs.
- Define retention windows for admin override history.
- Confirm that exported invoice metadata does not expose secrets.
- Confirm that support dashboards never show raw auth tokens or raw Stripe webhook payloads.

## Billing records

Billing records include invoice metadata, customer portal session IDs, support-visible customer references, webhook delivery summaries, and entitlement materialization timestamps. Retention should help support answer customer questions without turning operational logs into a second source of truth. `customer_id` may be retained for support lookup. Raw Stripe webhook payloads should have shorter retention than derived operational summaries.

Webhook replay summaries may include event type, replay flag, and processing status. They should not be used to recompute entitlement state. Durable idempotency remains an implementation concern for the webhook handler, not a retention policy decision.

## Auth records

Auth records include session validation outcomes, token refresh success or failure, workspace membership checks, invite token acceptance, and authorization materialization version. Raw tokens must never be logged. Session identifiers should be hashed. `authorization_details` should be summarized, not stored as a raw blob in security logs.

The review repeats "auth token" and "session cookie" many times because security needs those words. Retrieval should still prefer ADR 0005 and the auth-session OpenSpec change for implementation questions about the token handoff boundary.

## Support and audit records

Support records include admin override changes, invoice export access, customer portal help actions, invite resend actions, and manual notes. Audit records should preserve actor, subject, reason, timestamp, and related customer or workspace identifiers. They should not include raw payment information, raw tokens, or raw webhook payloads.

## Notification records

Notification delivery logs should retain notification key, recipient, delivery state, and source event. Idempotency for notification delivery is a local notification concern, not Stripe webhook idempotency. This is a common semantic trap: "idempotent delivery" can mean very different things depending on subsystem.

## Data minimization rules

- Store opaque IDs rather than raw provider payloads.
- Store hashed session identifiers rather than raw cookies.
- Store summarized authorization state rather than full `authorization_details`.
- Store `customer_id` only where support workflows require it.
- Store replay status only as operational metadata.
- Separate source-of-truth data from diagnostic logs.

## Review outcomes

The team agreed to keep webhook payload retention short, keep billing event summaries longer, hash session identifiers, and avoid copying authorization details into general-purpose logs. Product accepted that support dashboards may show "pending" when entitlement convergence is delayed. Security accepted that customer IDs are necessary for billing support but should not become a cross-product analytics identity.

## Why this file is in the eval

This review is a realistic distractor. It contains billing, webhook, replay, idempotency, entitlement, Stripe, customer, auth, token, session, customer portal, audit, support, and authorization vocabulary. It should rarely be selected for narrow engineering tasks, and when it is selected, precision should suffer. That makes it useful for measuring whether a retriever can compress repo intent without simply grabbing every file that shares common terms.

## Additional notes

During the review, several people asked whether webhook replay logs could be used to rebuild entitlement state. The answer was no. Logs help explain what happened; they do not define behavior. Another question asked whether auth/session logs could include enough authorization detail to debug billing access. The answer was to include version IDs and coarse feature counts, not raw authorization structures.

The review also considered whether customer portal sessions should have a separate retention rule. The conclusion was yes: portal session audit entries are useful for support, but their URLs expire and should not be retained as active links. Invoice export logs follow a separate finance retention policy.

Finally, the review called out local entitlement caching as a deprecated idea. Security preferred fewer caches of authorization state. That aligns with the superseded ADR and gives another reason not to revive the old local entitlement cache plan.

