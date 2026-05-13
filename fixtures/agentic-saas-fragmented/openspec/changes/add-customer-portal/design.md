# Design: Customer Portal

The customer portal endpoint requires an authenticated session and a workspace with a Stripe `customer_id`. It creates a Stripe-hosted portal session and returns the URL to the web app.

## Data flow

Auth/session validates the user. Billing reads the workspace `customer_id`. Stripe creates the portal session. After the user returns, subscription changes arrive as webhooks and later update entitlements.

## Ambiguity with entitlement sync

The portal design mentions webhook replay and idempotency because Stripe portal changes trigger webhooks. The portal endpoint itself does not process `stripe_event_id`, does not run entitlement sync, and does not write `authorization_details`.

