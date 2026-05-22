# Subtype-First Lane Classifier Plan

## Context

DevSpecs currently indexes intent artifacts well enough to show first-index value, but common non-intent markdown can still look like intent when it contains words such as plan, design, architecture, or requirements.

The lane taxonomy should help precision, but lane should be derived from concrete subtype evidence rather than guessed first.

## Goal

Add explicit subtype classifiers for high-confidence non-intent docs so they are quarantined from intent retrieval without broadening the product into "index everything."

## Initial Subtypes

Protocol:
- `agent_instruction` for `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, and similar repo-local agent rules
- `skill`
- `maintainer_policy`
- `ownership_policy`
- `governance_policy`
- `contribution_policy`
- `security_policy`
- `procedure`
- `runbook`
- `standard`

Template:
- `document_template`
- `prompt_template`
- `issue_template`
- `pull_request_template`

Model:
- `api_contract`
- `schema_model`
- `configuration`
- `workflow_definition`

## Implementation

1. Extend classifier metadata with a derived `mode` value.
2. Add three broad classifier models, `protocol`, `template`, and `model`, with subtype families driven by concrete path/title/body signals.
3. Keep existing intent classifiers intact.
4. For markdown adapter artifacts only, if a non-intent classifier wins, persist the artifact as `markdown_artifact` with the subtype family suffix.
5. Add retrieval metadata for `classifier_mode`.
6. Demote protocol/template/model artifacts in default intent retrieval unless the query explicitly asks for that lane/subtype.
7. Add tests for common false-positive sources, especially `CLAUDE.md`, `SKILL.md`, `MAINTAINERS.md`, templates, OpenAPI/schema/config/workflow docs.

## Non-Goals

- Do not discover all model files such as Terraform, Docker, or OpenAPI yet.
- Do not make lane detection a separate first-stage decision.
- Do not change ADR/OpenSpec/PRD/RFC/plan semantics unless a non-intent subtype wins with clear evidence.
- Do not claim public precision gains until validation reruns confirm.

## Success Criteria

- `CLAUDE.md` and `AGENTS.md` classify as protocol agent instructions, not plan/agent-note intent.
- `SKILL.md` classifies as protocol skill.
- `MAINTAINERS.md`, `CODEOWNERS.md`, `GOVERNANCE.md`, `CONTRIBUTING.md`, and `SECURITY.md` classify as protocol subtypes.
- `.github/PULL_REQUEST_TEMPLATE.md` and issue templates classify as template subtypes.
- OpenAPI/schema/config/workflow-shaped candidates classify as model subtypes when presented to the classifier.
- Existing intent classifier tests still pass.
- Retrieval demotes non-intent lanes for ordinary intent queries.
