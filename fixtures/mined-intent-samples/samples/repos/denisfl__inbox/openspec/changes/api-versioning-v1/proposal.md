## Why

The current API lives at `/api/` with no version prefix. Any breaking change (field rename, resource restructure, auth change) immediately breaks all clients — Telegram webhook, future MCP layer, any external integrations. There's no deprecation path and no way to evolve the API safely.

## What Changes

- Move all existing API endpoints under `/api/v1/` namespace
- Keep `/api/` routes as deprecated aliases pointing to v1 (with deprecation warning header)
- Add API versioning infrastructure (`Api::V1::BaseController`)
- Add rate limiting via `Rack::Attack` for API endpoints
- Document API with request/response examples

## Capabilities

### New Capabilities

- `api-v1-namespace`: Versioned API namespace with `/api/v1/` prefix and deprecation aliases for `/api/`
- `api-rate-limiting`: Request rate limiting via Rack::Attack for API endpoints

### Modified Capabilities

<!-- Existing API endpoints move under v1 namespace — functionally identical -->

## Impact

- **New files**: `app/controllers/api/v1/base_controller.rb`, `app/controllers/api/v1/*.rb` (moved controllers), `config/initializers/rack_attack.rb`
- **Modified files**: `config/routes.rb` (v1 namespace + deprecated aliases), `app/controllers/api/base_controller.rb` (deprecation header concern)
- **Dependencies**: `rack-attack` gem
- **Migration**: Telegram webhook URL must be updated from `/api/telegram/webhook` to `/api/v1/telegram/webhook` (configurable via ENV)
