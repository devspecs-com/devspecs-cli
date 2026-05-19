## Why

Technical decisions are currently scattered across `.project/architecture.md`, `.project/project-context.md`, `.project/about.md`, and individual OpenSpec change files. When a contributor (or the AI agent) asks "why do we use SQLite instead of PostgreSQL?" or "why was LLM classification removed?", they must search across multiple files. Architecture Decision Records (ADRs) provide a standardized, searchable format for recording and finding decisions.

Additionally, `AGENTS.md` lacks project constraints that AI agents need to follow, and there's no data model documentation.

## What Changes

- `docs/adr/` directory with initial ADRs extracted from existing decisions
- Standardized ADR template (MADR format)
- Updated `AGENTS.md` with project constraints and conventions
- `docs/data-model.md` documenting the SQLite schema and relationships

## Capabilities

### New Capabilities

- `adr-records`: Architecture Decision Records in `docs/adr/` using MADR template
- `data-model-docs`: Data model documentation with entity relationships and schema reference

### Modified Capabilities

<!-- AGENTS.md enhanced with project constraints -->

## Impact

- **New files**: `docs/adr/` directory with 5+ initial ADRs, `docs/adr/template.md`, `docs/data-model.md`
- **Modified files**: `AGENTS.md` (add constraints section)
- **Dependencies**: None
