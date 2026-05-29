---
status: draft
tags: [support, search, rfc]
---

# RFC 0009: Support Search Ranking

## Summary

Rank support search results for billing investigations by customer-facing language rather than implementation identifiers.

## Motivation

Support agents often search for Stripe, customer portal, invoice export, replay, session, auth, and entitlement terms in one broad query. This RFC is intentionally nearby but should not be selected for engineering implementation context.

## Proposal

Use support issue titles, customer message snippets, and product area tags as ranking features for the internal support search page.

## Detailed Design

The support search service may index `customer_id` for filtering, but it does not read `authorization_details`, does not touch webhook replay protection, and does not define source-code boundaries.

## Drawbacks

Ranking support search by product language can hide low-level implementation records.

## Alternatives

- Reuse the engineering document index for support search.
- Rank only by recency.
