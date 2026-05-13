---
status: superseded
tags: [entitlements, stale]
---

# ADR 0003: Superseded Local Entitlements Cache

## Status

Superseded by ADR 0001 and ADR 0002.

## Original decision

The original local entitlement caching plan proposed storing a local projection as the primary access source and reconciling Stripe asynchronously.

## Why superseded

The plan made offline access checks fast, but it placed the consistency boundary in a local cache instead of Stripe and did not solve webhook replay behavior. It should be read only when someone asks why the local entitlement caching plan was abandoned.

