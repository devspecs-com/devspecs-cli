## ADDED Requirements

### Requirement: Durable webhook replay protection

Billing webhook handling MUST store each accepted `stripe_event_id` before entitlement side effects run.

#### Scenario: duplicate Stripe event arrives

- **GIVEN** `stripe_event_id` already exists in `billing_webhook_events`
- **WHEN** Stripe retries the same event
- **THEN** the API responds successfully
- **AND** `entitlement_sync` is not executed again

### Requirement: Customer entitlement sync

Accepted subscription events MUST sync entitlements by `customer_id` and refresh `authorization_details`.

