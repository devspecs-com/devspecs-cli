## ADDED Requirements

### Requirement: Versioned API namespace

All API endpoints SHALL be accessible under the `/api/v1/` prefix.

#### Scenario: V1 endpoints respond

- **WHEN** a client sends a request to `/api/v1/documents`
- **THEN** the system SHALL return the same response as the current `/api/documents`

#### Scenario: V1 base controller

- **GIVEN** the `Api::V1::BaseController` exists
- **THEN** it SHALL inherit from `Api::BaseController` and provide the v1 request context

### Requirement: Deprecation aliases

The system SHALL maintain `/api/` routes as deprecated aliases pointing to v1 handlers.

#### Scenario: Deprecated endpoint still works

- **WHEN** a client sends a request to `/api/documents`
- **THEN** the system SHALL return the v1 response with a `Deprecation` HTTP header warning

#### Scenario: Deprecation header present

- **WHEN** a client uses a `/api/` (non-versioned) endpoint
- **THEN** the response SHALL include `Deprecation: true` and `Sunset` headers indicating migration timeline

### Requirement: Telegram webhook path update

The Telegram webhook URL SHALL be configurable and default to `/api/v1/telegram/webhook`.

#### Scenario: Webhook receives update at v1 path

- **WHEN** Telegram sends an update to `/api/v1/telegram/webhook`
- **THEN** the system SHALL process it normally via `ProcessTelegramUpdateJob`
