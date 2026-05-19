## Context

Technical decisions are documented in `.project/architecture.md` (5 finalized decisions), `.project/project-context.md` (5 resolved decisions), and `.project/about.md` (key decisions section). These overlap and are not in a standardized searchable format. `AGENTS.md` exists but is nearly empty. No data model documentation exists beyond `db/schema.rb`.

## Decisions

### 1. MADR (Markdown Any Decision Records) format

**Rationale**: MADR is the most widely adopted ADR format. Simple markdown, easy to read, grep-friendly. Each ADR is a numbered file: `0001-use-sqlite.md`.

**Template**:

```markdown
# ADR-NNNN: Title

## Status

Accepted | Deprecated | Superseded by ADR-NNNN

## Context

What is the issue that we're seeing that is motivating this decision?

## Decision

What is the change that we're proposing and/or doing?

## Consequences

What becomes easier or more difficult to do because of this change?
```

### 2. Five initial ADRs extracted from existing docs

1. **ADR-0001: Use SQLite for single-user data store** (from architecture.md §2)
2. **ADR-0002: Use Parakeet v3 for local audio transcription** (from architecture.md §1)
3. **ADR-0003: Remove LLM classification from pipeline** (from project-context.md)
4. **ADR-0004: Minimal token-based authentication** (from architecture.md §3)
5. **ADR-0005: Adopt SolidQueue over Sidekiq** (from project-context.md, current Gemfile)

### 3. Data model docs generated from schema.rb

**Rationale**: `docs/data-model.md` is a human-readable reference generated from `db/schema.rb` with added relationship annotations and business context. Not auto-generated — maintained manually to include intent and constraints that schema alone doesn't convey.

### 4. AGENTS.md expanded with project constraints

**Rationale**: AI agents read `AGENTS.md` before working on code. Adding constraints (no version downgrades, no emoji, testing requirements, env-config patterns) prevents common agent mistakes.

## Risks

1. **ADR drift**: ADRs become outdated if not updated when decisions change. Mitigate by reviewing ADRs during architecture changes.
2. **Data model drift**: `docs/data-model.md` can diverge from actual schema. Mitigate by adding a CI check or a note to update during migrations.

## Implementation order

1. Create `docs/adr/template.md`
2. Write 5 initial ADRs
3. Create `docs/data-model.md` from `db/schema.rb`
4. Update `AGENTS.md` with project constraints
5. Add ADR creation guidance to CONTRIBUTING.md
