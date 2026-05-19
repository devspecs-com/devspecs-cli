# PRD: Integrate x-search skill for X/Twitter research

## Metadata
- **Type**: Feature
- **Priority**: Medium
- **Severity**: N/A
- **Estimated Complexity**: 5
- **Created**: 2026-02-08
- **Status**: Ready for Implementation

## Overview

### Problem Statement
The sdlc-plugin lacks a skill for searching X/Twitter for real-time developer discourse, product feedback, and community sentiment. An open-source implementation exists at [rohunvora/x-research-skill](https://github.com/rohunvora/x-research-skill) that provides a Bun CLI wrapping the X API v2 with search, thread, profile, watchlist, and caching. It needs adaptation to match sdlc-plugin conventions (pure-markdown SKILL.md, TypeScript under `utils/`, env-var-only auth, no hardcoded paths).

### Goals & Objectives
1. Add a working `/x-search` skill that enables X/Twitter research from Claude Code
2. Adapt all source code to match sdlc-plugin conventions (file layout, frontmatter, auth pattern)
3. Fix the known thread root tweet bug during integration
4. Default to markdown output (not Telegram format) for Claude Code context
5. Remove author-specific hardcoded paths and personal references

### Success Metrics
- **Primary Metric**: `bun run utils/x-search/x-search.ts search "test" --markdown` returns formatted results with valid X_BEARER_TOKEN
- **Secondary Metrics**: All 6 CLI commands (search, thread, profile, tweet, watchlist, cache) function correctly
- **Quality Gates**: `bun run validate` passes; no hardcoded `~/clawd/` or `~/.config/env/` paths remain

## User Stories

### Story 1: Search X for topic discourse
- **As a**: developer using Claude Code
- **I want**: to search X/Twitter for what people are saying about a topic
- **So that**: I can get real-time community sentiment, expert opinions, and linked resources
- **Acceptance Criteria**:
  - [ ] `/x-search "topic"` triggers the skill and runs search
  - [ ] Results show author, engagement metrics, text, and tweet URL
  - [ ] Output defaults to markdown format

### Story 2: Follow a conversation thread
- **As a**: developer researching a discussion
- **I want**: to fetch a full X conversation thread by tweet ID
- **So that**: I can read the complete context of a discussion
- **Acceptance Criteria**:
  - [ ] `thread <tweet_id>` fetches root tweet + replies
  - [ ] Root tweet is correctly included (bug fix)

### Story 3: Monitor accounts via watchlist
- **As a**: developer tracking key voices
- **I want**: to maintain a watchlist of X accounts and check their recent posts
- **So that**: I can stay current on important accounts without manual searching
- **Acceptance Criteria**:
  - [ ] `watchlist add/remove/check` commands work
  - [ ] Watchlist persists in `utils/x-search/data/watchlist.json`

## Requirements

### Functional Requirements

1. **FR-1**: SKILL.md prompt follows sdlc-plugin conventions (frontmatter with name/description/argument-hint, Priorities, Goal, Constraints, References, Arguments sections)
   - Priority: Must Have

2. **FR-2**: CLI supports all 6 commands: search, thread, profile, tweet, watchlist, cache
   - Priority: Must Have

<!-- Addressed: [Gemini] Ambiguous markdown output spec — defined schema -->
3. **FR-3**: Default output format is markdown (not Telegram)
   - Details: `--telegram` flag for legacy format, `--markdown` becomes default, `--json` for raw
   - Markdown schema: Each tweet must include `@username`, date, text (truncated to 200 chars), engagement metrics (likes, impressions), and tweet URL. Research doc format includes theme grouping, metadata section with query and tweet count. No raw JSON dumps.
   - Priority: Must Have

4. **FR-4**: `--save` writes to current working directory as `x-research-{slug}-{date}.md`
   - Priority: Must Have

5. **FR-5**: Authentication uses `X_BEARER_TOKEN` env var only, no file fallback
   - Priority: Must Have

6. **FR-6**: Thread command correctly fetches and includes root tweet
   - Details: Fix `api.ts:226-232` bug where single tweet response parsing is incomplete
   - Priority: Must Have

7. **FR-7**: Cache lives in `utils/x-search/data/cache/` with 15min TTL
   - Priority: Should Have

8. **FR-8**: X API reference doc available at `skills/x-search/references/x-api.md`
   - Priority: Should Have

### Non-Functional Requirements

1. **NFR-1**: Rate limiting
   - Requirement: Respect X API rate limits (450 req/15min)
   - Target: 350ms delay between API calls (existing implementation)
   - Measurement: No 429 errors in normal usage

2. **NFR-2**: Error messages
   - Requirement: Missing X_BEARER_TOKEN produces clear setup instructions
   - Target: Error message includes developer portal URL
   - Measurement: Running without token shows actionable error

<!-- Addressed: [Consensus] API error state handling for 401/403/429 -->
3. **NFR-3**: API error state handling
   - Requirement: Handle 401 (invalid token), 403 (insufficient scope), and 429 (rate limit) with distinct, actionable messages
   - Target: 401/403 prints "Invalid or expired X_BEARER_TOKEN" with portal URL; 429 prints reset time from `x-rate-limit-reset` header
   - Measurement: CLI exits with non-zero code and clear message for each error type

### Technical Requirements
- **Stack**: Bun + TypeScript (existing pattern)
- **Dependencies**: None new (uses Bun built-ins: fetch, crypto, fs)
- **Architecture**: SKILL.md in `skills/x-search/`, CLI + libs in `utils/x-search/`
- **Data Model**: `Tweet` interface with id, text, author, metrics, urls, mentions, hashtags
- **API Contracts**: X API v2 `/tweets/search/recent`, `/tweets/{id}`, `/users/by/username/{username}`

## Scope

### In Scope
- Copy and adapt all source files from rohunvora/x-research-skill
- Rewrite SKILL.md to match sdlc-plugin conventions
- Adapt TypeScript code: remove hardcoded paths, env-only auth, fix thread bug
- Change default output to markdown
- Change `--save` to write to CWD
- Add `.gitignore` entries for cache and watchlist data
- Copy `x-api.md` reference to `skills/x-search/references/`

### Out of Scope
- MCP server wrapping (x-search is a CLI tool, not an MCP server)
- Adding X_BEARER_TOKEN to `.mcp.json` (it's not an MCP server)
- Posting/interacting with X (read-only)
- X API Full Archive search (requires Enterprise tier)
- Automated testing of API calls (requires live X API credentials)

### Future Considerations
- MCP server wrapper for x-search (similar to perplexity-mcp pattern)
- Integration with `/sdlc:research` command for multi-source research

## Impact Analysis

### Affected Areas
- `skills/` — new `x-search/` directory
- `utils/` — new `x-search/` directory
- `.gitignore` — new entries for cache/watchlist data

### Users Affected
- Plugin users who want X/Twitter research capabilities (opt-in via X_BEARER_TOKEN setup)

### System Impact
- **Performance**: No impact on existing functionality; CLI only runs when invoked
- **Security**: Bearer token read from env only; no file system scanning for secrets
- **Data Integrity**: Cache and watchlist are local, disposable data

### Dependencies
- **Upstream**: X API v2 (external service, $200/mo Basic tier)
- **Downstream**: None
- **External**: Bun runtime (already required by plugin)

### Breaking Changes
- [x] **None** — purely additive feature

## Solution Design

### Approach
Port all files from the external repository, adapting to sdlc-plugin conventions:

1. **SKILL.md** — Rewrite from scratch using plugin frontmatter schema and section conventions. Keep the agentic research loop content but restructure into Goal/Constraints/Workflow sections. Remove author-specific references (heartbeat, "Flag to Frank").

2. **TypeScript CLI** — Copy `x-search.ts` + `lib/` to `utils/x-search/`. Modifications:
   - Remove `global.env` file fallback in `getToken()`
   - Change default output from Telegram to markdown format
   - Change `--save` path from `~/clawd/drafts/` to CWD
   - Fix thread root tweet parsing bug
   - Update error message for missing token

3. **Data/References** — Copy `data/` structure and `x-api.md` reference to appropriate locations.

### Alternatives Considered

1. **MCP server wrapper instead of CLI**
   - Pros: Tighter integration with Claude Code tool system
   - Cons: Significantly more work; CLI already works well via Bash tool
   - Why rejected: CLI approach matches the source design and works immediately

2. **Inline the API calls in SKILL.md (no TypeScript)**
   - Pros: Pure markdown skill, no utils/ code
   - Cons: Claude would need to construct curl commands; slower, error-prone, no caching
   - Why rejected: CLI provides caching, formatting, and reliable execution

### Data Model Changes
None — file-based cache and watchlist (JSON files in `data/`)

### API Changes
None — wraps external X API v2

### UI/UX Changes
None — CLI output consumed by Claude

## Implementation Plan

### Phase 1: File Structure and SKILL.md
**Complexity**: 3 | **Priority**: High

<!-- Addressed: [Codex] License verification -->
- [ ] Verify upstream repository license (check for LICENSE file or package.json license field); add attribution comment in copied files if required
- [ ] Create `skills/x-search/` directory
- [ ] Write `skills/x-search/SKILL.md` with sdlc-plugin conventions (frontmatter, Priorities, Goal, Constraints, CLI Tool section, Research Loop, References, Arguments)
- [ ] Copy `references/x-api.md` to `skills/x-search/references/x-api.md`

### Phase 2: TypeScript CLI Adaptation
**Complexity**: 4 | **Priority**: High

- [ ] Create `utils/x-search/` directory structure (`lib/`, `data/`, `data/cache/`)
- [ ] Copy and adapt `x-search.ts`: change `DRAFTS_DIR` to use `process.cwd()`, swap default format from Telegram to markdown
- [ ] Copy and adapt `lib/api.ts`: remove `global.env` file fallback from `getToken()`, update error message for missing token
<!-- Addressed: [Consensus] Add 401/403/429 handling in apiGet() -->
- [ ] Add explicit 401/403 handling in `apiGet()` with actionable error message ("Invalid or expired X_BEARER_TOKEN"); surface `x-rate-limit-reset` header in 429 error message
- [ ] Fix thread root tweet bug in `lib/api.ts` `thread()` function — handle single tweet API response correctly
- [ ] Copy `lib/cache.ts` as-is (paths use `import.meta.dir` relative, which works)
- [ ] Copy and adapt `lib/format.ts` as-is (no path changes needed)

### Phase 3: Data Files and Gitignore
**Complexity**: 1 | **Priority**: Medium

- [ ] Copy `data/watchlist.example.json` to `utils/x-search/data/`
- [ ] Create `utils/x-search/data/cache/.gitkeep`
- [ ] Append to sdlc-plugin `.gitignore`: `utils/x-search/data/cache/*.json` and `utils/x-search/data/watchlist.json`

### Phase 4: Validation
**Complexity**: 2 | **Priority**: High

- [ ] Run `bun run validate` to confirm plugin manifest still passes
- [ ] Verify no hardcoded `~/clawd/` or `~/.config/env/` paths in any copied file
- [ ] Verify SKILL.md frontmatter has name, description, argument-hint fields
- [ ] Dry-run CLI with `bun run utils/x-search/x-search.ts` (shows usage, no API call)
- [ ] Verify `--save` writes to CWD (check code path, no live API needed)

## Relevant Files

### Existing Files
- `.gitignore` — needs new entries for x-search data
- `skills/gemini/SKILL.md` — frontmatter convention reference
- `skills/codex/SKILL.md` — frontmatter convention reference
- `.mcp.json` — env var pattern reference (not modified)
- `utils/perplexity-mcp/` — precedent for TypeScript utils

### New Files
- `skills/x-search/SKILL.md` — skill prompt definition
- `skills/x-search/references/x-api.md` — X API endpoint reference
- `utils/x-search/x-search.ts` — CLI entry point
- `utils/x-search/lib/api.ts` — X API wrapper
- `utils/x-search/lib/cache.ts` — file-based cache
- `utils/x-search/lib/format.ts` — output formatters
- `utils/x-search/data/watchlist.example.json` — watchlist template
- `utils/x-search/data/cache/.gitkeep` — cache directory placeholder
- `utils/x-search/tests/parse-tweets.test.ts` — fixture-based unit tests
- `utils/x-search/tests/fixtures/thread-response.json` — captured thread API response
- `utils/x-search/tests/fixtures/search-response.json` — captured search API response
- `utils/x-search/tests/fixtures/single-tweet-response.json` — captured single tweet response

### Test Files
<!-- Addressed: [Gemini] Missing regression tests for bug fix — add fixture-based unit tests -->
- `utils/x-search/tests/parse-tweets.test.ts` — fixture-based tests for `parseTweets` and thread root tweet handling
- `utils/x-search/tests/fixtures/` — captured JSON responses for thread, search, single tweet

## Testing Strategy

<!-- Addressed: [Gemini] Regression tests for thread bug fix using mock fixtures -->
### Unit Tests
- Fixture-based tests for `parseTweets()` function to verify correct extraction of tweet data from API responses
- Thread root tweet test: feed single-tweet API response fixture to verify root tweet is correctly parsed and included
- Search response test: verify deduplication, sorting, and metric extraction from multi-page fixture

### Integration Tests
- Not applicable — requires live X API credentials ($200/mo)

### E2E Tests
- Not applicable — same credential requirement

### Manual Test Cases

1. **Test Case**: CLI shows usage without API call
   - Steps: `bun run utils/x-search/x-search.ts`
   - Expected: Prints usage help, exits 0

2. **Test Case**: Missing token produces clear error
   - Steps: `unset X_BEARER_TOKEN && bun run utils/x-search/x-search.ts search "test"`
   - Expected: Error message with developer portal URL

3. **Test Case**: No hardcoded paths remain
   - Steps: `grep -r "clawd\|global\.env" utils/x-search/ skills/x-search/`
   - Expected: No matches

4. **Test Case**: Plugin validation passes
   - Steps: `bun run validate`
   - Expected: Validation successful

5. **Test Case**: SKILL.md frontmatter is valid
   - Steps: Read `skills/x-search/SKILL.md`, check for name/description/argument-hint
   - Expected: All three fields present

<!-- Addressed: [Codex] Manual tests omit critical error paths -->
6. **Test Case**: Watchlist add/remove/check works
   - Steps: `bun run utils/x-search/x-search.ts watchlist add testuser "test note"` then `watchlist` then `watchlist remove testuser`
   - Expected: Account added, listed, then removed. Watchlist file updated.

7. **Test Case**: Unit tests pass
   - Steps: `bun test utils/x-search/tests/`
   - Expected: All fixture-based parse tests pass

## Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| X API changes break response parsing | Low | Medium | `x-api.md` reference doc documents expected format; `parseTweets` handles missing fields gracefully |
| Rate limiting during heavy research sessions | Medium | Low | 350ms delay between calls; 15min cache prevents duplicate queries |
| Bearer token leaked in output | Low | High | Token only used in HTTP headers; never logged or output |

### Business Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| X API cost surprises | Low | Low | Cache reduces API calls; SKILL.md documents ~$0.005/tweet cost |

### Mitigation Strategy
The skill is purely additive and opt-in (requires X_BEARER_TOKEN setup). All data is local and disposable (cache, watchlist). Rollback is simply deleting the `skills/x-search/` and `utils/x-search/` directories.

## Rollback Strategy

### Rollback Steps
1. Delete `skills/x-search/` directory
2. Delete `utils/x-search/` directory
3. Remove `.gitignore` entries for x-search

### Rollback Conditions
- X API v2 becomes unavailable or pricing changes dramatically
- Skill causes issues with plugin validation

## Validation Commands

<!-- stripped fenced code block: bash -->

## Acceptance Criteria

- [ ] `skills/x-search/SKILL.md` exists with valid frontmatter (name, description, argument-hint)
- [ ] SKILL.md has Priorities, Goal, Constraints, CLI Tool, Research Loop, References, Arguments sections
- [ ] `utils/x-search/x-search.ts` runs and shows usage with `bun run`
- [ ] No `~/clawd/`, `~/.config/env/`, or "Frank" references in any file
- [ ] `lib/api.ts` uses env var only for token, with actionable error message
- [ ] Default output format is markdown (not Telegram)
- [ ] `--save` writes to CWD
- [ ] Thread root tweet bug is fixed in `lib/api.ts`
- [ ] `.gitignore` includes cache and watchlist entries
- [ ] `bun run validate` passes
- [ ] `x-api.md` reference exists at `skills/x-search/references/x-api.md`
- [ ] `apiGet()` handles 401/403 with actionable error message and non-zero exit
- [ ] `apiGet()` handles 429 with rate-limit reset time in error message
- [ ] Fixture-based unit tests pass for `parseTweets` and thread root tweet
- [ ] Upstream license verified and attributed if required

## Dependencies

### New Dependencies
- None (uses Bun built-ins: fetch, crypto, fs, path)

### Dependency Updates
- None

## Notes & Context

### Additional Context
- Source repository: https://github.com/rohunvora/x-research-skill (6 commits, 49 stars, MIT implied by public repo)
- Research doc: `research/research-x-search-skill-integration.md`
- The source was designed for a different plugin system (OpenClaw/personal setup) so requires path and convention adaptation

### Assumptions
- User has an X API Basic tier subscription ($200/mo) with a bearer token
- Bun runtime is installed (already required by sdlc-plugin)
- The skill will only be used for read-only X research (no posting)

### Constraints
- X API Basic tier only covers last 7 days of tweets
- `min_likes`/`min_retweets` search operators unavailable on Basic tier (filtered post-hoc)
- Max 100 tweets per API request, max 5 pages (500 tweets per search)

### Related Tasks/Issues
- None

### References
- [X API v2 Documentation](https://developer.x.com/en/docs/twitter-api)
- [rohunvora/x-research-skill](https://github.com/rohunvora/x-research-skill)
- `research/research-x-search-skill-integration.md`

### Open Questions
- None (all resolved during interview phase)

## Blindspot Review

**Reviewers**: GPT-5.2-Codex (xhigh), Gemini 3 Pro
**Date**: 2026-02-08
**Plan Readiness**: Ready

### Addressed Concerns
- [Consensus] API error state handling (401/403/429) not defined beyond missing token → Added NFR-3, updated Phase 2 with `apiGet()` error handling task
- [Gemini, High] Missing regression tests for thread root tweet bug fix → Added fixture-based unit tests in Testing Strategy, new test files in Relevant Files
- [Gemini, Medium] Ambiguous markdown output spec → Added schema definition to FR-3
- [Codex, Medium] License verification missing → Added verification step to Phase 1
- [Codex, Medium] Manual tests omit critical paths → Added watchlist and unit test cases

### Acknowledged but Deferred
- [Codex, Medium] Network timeouts and transient failure retries → Source has no timeout handling; adding retry logic is scope creep for initial port. Track as follow-up.
- [Codex, Medium] Save-to-CWD filename collisions → Date-based slugs make collisions unlikely; overwrite is acceptable default. Track if reported.

### Dismissed
- [Gemini, High] Data persistence in source tree anti-pattern → This is a personal plugin installed via symlink, not distributed via npm. Writing to `utils/x-search/data/` is the same pattern as the source repo and matches user's explicit choice during interview.
- [Codex, High] Skill discovery/registration dependency → sdlc-plugin auto-discovers skills by directory convention (`skills/*/SKILL.md`). No manifest or registry to update.
- [Codex, Medium] Watchlist/cache bootstrap and corruption → Source code already handles missing files gracefully (`loadWatchlist()` returns empty if file missing; cache returns null on parse error).
- [Codex, Medium] Thread pagination for large threads → Already handled via `--pages` parameter (up to 5 pages, 500 tweets).
- [Codex, High] API field/expansion requirements not ensured → Source already defines `FIELDS` constant with all required tweet.fields, expansions, and user.fields at `api.ts:106-107`.
- [Gemini, Low] Execution path reliability / SKILL.md command paths → SKILL.md uses `$CLAUDE_PLUGIN_ROOT` or Glob-based path resolution, matching existing skill patterns (gemini, codex).
