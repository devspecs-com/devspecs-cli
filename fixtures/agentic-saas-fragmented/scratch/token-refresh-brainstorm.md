# Token Refresh Brainstorm

Status: stale scratch.

This scratch file mixes auth token refresh, session cookies, billing dashboard refresh behavior, customer portal redirects, and entitlement-derived `authorization_details`. It is intentionally similar to the auth-session boundary work but should usually be excluded because ADR 0005 supersedes it.

Ideas in this brainstorm:

- Refresh auth tokens from the billing dashboard route.
- Store a `customer_id` in a browser-visible token claim.
- Ask the webhook handler to invalidate sessions after subscription changes.
- Retry token refresh after customer portal redirects.

All of those ideas were rejected. Use ADR 0005 and the `refactor-auth-session` OpenSpec change instead.

