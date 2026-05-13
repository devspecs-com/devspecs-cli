## ADDED Requirements

### Requirement: Server-owned token refresh

Auth token refresh MUST happen behind the API session boundary.

#### Scenario: billing dashboard route loads

- **GIVEN** a browser has a valid opaque session cookie
- **WHEN** the billing dashboard requests authorization state
- **THEN** the API validates the session
- **AND** loads `authorization_details`
- **AND** does not expose a long-lived bearer token to the browser

### Requirement: Portal redirect session check

Customer portal redirects MUST re-check the auth session before creating or resuming billing UI state.

