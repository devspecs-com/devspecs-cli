## Tasks

### Group 1: V1 namespace infrastructure

- [ ] 1.1 Create `app/controllers/api/v1/base_controller.rb` inheriting from `Api::BaseController`
- [ ] 1.2 Create `app/controllers/api/v1/` directory structure

### Group 2: Move controllers to v1

- [ ] 2.1 Move `Api::DocumentsController` → `Api::V1::DocumentsController`
- [ ] 2.2 Move `Api::BlocksController` → `Api::V1::BlocksController`
- [ ] 2.3 Move `Api::TelegramController` → `Api::V1::TelegramController`
- [ ] 2.4 Move `Api::TagsController` → `Api::V1::TagsController`
- [ ] 2.5 Move `Api::DocumentTagsController` → `Api::V1::DocumentTagsController`
- [ ] 2.6 Move `Api::TaskTagsController` → `Api::V1::TaskTagsController`
- [ ] 2.7 Move `Api::CalendarEventTagsController` → `Api::V1::CalendarEventTagsController`
- [ ] 2.8 Move `Api::UploadsController` → `Api::V1::UploadsController`

### Group 3: Routes update

- [ ] 3.1 Wrap all existing API routes inside `namespace :v1 do ... end` within `namespace :api`
- [ ] 3.2 Add deprecated `/api/` aliases that route to v1 controllers
- [ ] 3.3 Update Telegram webhook route to `/api/v1/telegram/webhook`

### Group 4: Deprecation warning concern

- [ ] 4.1 Create `app/controllers/concerns/deprecation_warning.rb` that adds `Deprecation` and `Sunset` response headers
- [ ] 4.2 Include concern in deprecated route handlers
- [ ] 4.3 Log deprecated endpoint usage at WARN level

### Group 5: Rate limiting

- [ ] 5.1 Add `rack-attack` gem to Gemfile
- [ ] 5.2 Create `config/initializers/rack_attack.rb` with throttle rules (60 req/min default)
- [ ] 5.3 Configure per-IP and per-token throttling
- [ ] 5.4 Return 429 with `Retry-After` header on limit exceeded
- [ ] 5.5 Add `API_RATE_LIMIT` ENV variable support

### Group 6: Configuration

- [ ] 6.1 Add to `.env.example`: `API_RATE_LIMIT`
- [ ] 6.2 Update `TELEGRAM_WEBHOOK_URL` example to use `/api/v1/telegram/webhook`
- [ ] 6.3 Document migration guide in `docs/api-v1-migration.md`

### Group 7: Tests

- [ ] 7.1 Update all API request specs to use `/api/v1/` paths
- [ ] 7.2 Test deprecated `/api/` endpoints return correct response with deprecation headers
- [ ] 7.3 Test rate limiting returns 429 when threshold exceeded
- [ ] 7.4 Test Telegram webhook works at new v1 path
