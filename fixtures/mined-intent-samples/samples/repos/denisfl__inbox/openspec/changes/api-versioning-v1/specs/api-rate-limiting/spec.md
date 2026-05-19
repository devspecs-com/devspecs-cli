## ADDED Requirements

### Requirement: API rate limiting

The system SHALL enforce request rate limits on API endpoints to prevent abuse.

#### Scenario: Normal usage within limits

- **WHEN** a client sends requests within the rate limit (default: 60 requests/minute)
- **THEN** all requests SHALL be processed normally

#### Scenario: Rate limit exceeded

- **WHEN** a client exceeds the rate limit
- **THEN** the system SHALL return HTTP 429 (Too Many Requests) with a `Retry-After` header

#### Scenario: Rate limit scoping

- **GIVEN** rate limiting is configured
- **THEN** limits SHALL be applied per IP address for unauthenticated requests and per API token for authenticated requests

### Requirement: Rate limit configuration

Rate limits SHALL be configurable via environment variables.

#### Scenario: Custom rate limit

- **GIVEN** `API_RATE_LIMIT` is set to `120`
- **THEN** the system SHALL allow 120 requests per minute per client
