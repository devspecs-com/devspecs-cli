## Context

The current API at `/api/` has 9 controllers (base, blocks, calendar_event_tags, document_tags, documents, tags, task_tags, telegram, uploads). All inherit from `Api::BaseController` which provides token authentication and error handling. There's no version prefix — breaking changes would affect all consumers simultaneously.

## Decisions

### 1. Namespace under `/api/v1/` via Rails route namespace

**Rationale**: Rails' `namespace :v1` within `namespace :api` creates a clean `Api::V1::` module namespace. Controllers move to `app/controllers/api/v1/`. Existing `Api::BaseController` remains as shared infrastructure.

**Structure**:

```
app/controllers/api/
  base_controller.rb         # Shared auth, error handling (unchanged)
  v1/
    base_controller.rb       # < Api::BaseController (v1-specific if needed)
    documents_controller.rb  # Moved from api/
    telegram_controller.rb   # Moved from api/
    ...
```

### 2. Deprecated `/api/` aliases via route `match` redirects

**Rationale**: To avoid breaking existing clients, old `/api/` routes forward to v1 controllers and inject `Deprecation` response headers via a `DeprecationWarning` concern. This gives consumers time to migrate.

**Alternative rejected**: HTTP 301 redirects — would break POST/PATCH/DELETE Telegram webhooks.

### 3. Rack::Attack for rate limiting

**Rationale**: `rack-attack` is the standard Rails solution for request throttling. Config via initializer, limits per IP or per token, ENV-configurable thresholds. Returns 429 with `Retry-After`.

**Key config**: Default 60 req/min per client, configurable via `API_RATE_LIMIT` ENV.

### 4. Deprecation timeline: 6 months

**Rationale**: `Sunset` header (RFC 8594) communicates removal date. Old `/api/` routes will be removed after 6 months of v1 availability. Logging tracks usage of deprecated endpoints.

## Risks

1. **Telegram webhook URL change**: Must update Telegram bot webhook configuration after deploy. Mitigate by supporting both paths during transition.
2. **Test suite updates**: All API request specs reference `/api/` paths — must update to `/api/v1/`. Mitigate with find-and-replace.
3. **Client migration**: Any external script using `/api/` must update. Mitigate with the 6-month deprecation period.

## Implementation order

1. Create `Api::V1::BaseController` and `api/v1/` directory
2. Move/copy all existing API controllers to `api/v1/` namespace
3. Update `config/routes.rb` with v1 namespace
4. Add deprecated route aliases with `DeprecationWarning` concern
5. Add `rack-attack` gem and rate limiting initializer
6. Update Telegram webhook path (ENV-configurable)
7. Update all API request specs
8. Document API migration guide
