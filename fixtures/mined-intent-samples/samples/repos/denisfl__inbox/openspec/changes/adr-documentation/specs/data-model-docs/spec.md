## ADDED Requirements

### Requirement: AGENTS.md constraints

`AGENTS.md` SHALL document project constraints that AI agents must follow.

#### Scenario: Constraints present

- **GIVEN** `AGENTS.md` at the repository root
- **THEN** it SHALL include: version constraints (no downgrades), forbidden patterns (no emoji, no silent fallbacks), required patterns (explicit error handling, ENV-based config), testing requirements (RSpec, 80% coverage), and deployment constraints (Docker, SQLite)

#### Scenario: Agent reads constraints

- **WHEN** an AI agent begins work on the repository
- **THEN** it SHALL consult `AGENTS.md` for project-specific rules before making changes
