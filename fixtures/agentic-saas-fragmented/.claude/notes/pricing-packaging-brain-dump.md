---
status: stale
source: claude-notes
tags: [pricing, packaging, billing]
---

# Claude Notes: Pricing Packaging Brain Dump

This stale note collects pricing and packaging ideas. It mentions billing, Stripe, customer portal, entitlements, admin overrides, auth sessions, token limits, and support workflows because pricing touches many surfaces.

## Ideas

- Team plan with per-seat billing.
- Enterprise plan with annual invoice export.
- Trial plan with temporary admin override.
- Usage-based add-on tracked outside entitlement sync.
- Customer portal upgrade prompts.
- Billing dashboard plan comparison.
- Support audit timeline for pricing exceptions.

## Rejected coupling

Do not let pricing experiments write `authorization_details` directly. Do not let customer portal UX bypass webhook processing. Do not treat auth token refresh as a pricing concern. Do not use `customer_id` as a product analytics identifier outside billing contexts.

## Why this is noisy

The note shares terms with the active billing webhook hardening scenario, but it is roadmap brainstorming. It should not be included for implementation context unless the query explicitly asks about pricing or packaging.

