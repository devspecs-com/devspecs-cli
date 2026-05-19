## Tasks

### Group 1: ADR infrastructure

- [ ] 1.1 Create `docs/adr/` directory
- [ ] 1.2 Create `docs/adr/template.md` with MADR format template
- [ ] 1.3 Create `docs/adr/README.md` explaining ADR purpose and how to create new ones

### Group 2: Initial ADRs

- [ ] 2.1 `docs/adr/0001-use-sqlite.md` — SQLite for single-user, WAL mode, PostgreSQL migration path
- [ ] 2.2 `docs/adr/0002-use-parakeet-v3.md` — Local Parakeet v3 over cloud APIs, onnx-asr, 25 languages
- [ ] 2.3 `docs/adr/0003-remove-llm-classification.md` — LLM intent classification removed, replaced by manual/rule-based routing
- [ ] 2.4 `docs/adr/0004-token-based-auth.md` — Minimal token auth for MVP, single user, local network assumption
- [ ] 2.5 `docs/adr/0005-adopt-solidqueue.md` — SolidQueue over Sidekiq, removes Redis dependency, built into Rails 8

### Group 3: Data model documentation

- [ ] 3.1 Create `docs/data-model.md` with entity descriptions, relationships, and key fields
- [ ] 3.2 Include ER diagram (text-based Mermaid format)
- [ ] 3.3 Document join tables and their purposes (document_tags, task_tags, calendar_event_tags)

### Group 4: AGENTS.md update

- [ ] 4.1 Add "Project Constraints" section: version policy, forbidden patterns, required patterns
- [ ] 4.2 Add "Testing Requirements" section: RSpec, 80% coverage, WebMock for external services
- [ ] 4.3 Add "Architecture" section: key decisions reference (point to ADRs), deployment model

### Group 5: Contributing guide update

- [ ] 5.1 Add ADR section to `CONTRIBUTING.md` explaining when and how to create new ADRs
