# Tasks: Refactor Auth Session

- [x] Accept ADR 0005 as the auth/session boundary.
- [ ] Add `services/api/src/auth/tokens.ts` helper tests.
- [ ] Update session middleware to load `authorization_details` after token refresh.
- [ ] Remove billing dashboard token refresh hooks.
- [ ] Verify customer portal redirects re-check session state.
- [ ] Add regression tests for expired token and active entitlement state.

