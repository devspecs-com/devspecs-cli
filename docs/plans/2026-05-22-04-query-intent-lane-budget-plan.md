# Query Intent Routing And Lane Budget Plan

Date: 2026-05-22

Order: 4

Depends on:

- [0] Authority priors
- [1] Duplicate / variant collapse
- [2] Section indexing + packing
- [3] Tiered output

## Goal

Formalize lightweight query intent routing and lane budgets so common retrieval cases work without an overbuilt planner.

The default behavior should solve most common agent queries while remaining understandable and auditable.

## Query Modes

Start with a small closed set:

- implementation_or_resume
- product_background
- decision_or_rationale
- proposal_or_design
- roadmap_or_plan
- protocol_or_instruction
- template_request
- model_or_contract
- broad_context

The router should be deterministic at first, with optional future LLM/embedding assistance.

## Lane Budgets

Each query mode should define soft budgets by lane/subtype.

Example:

- `product_background`
  - primary: PRD/product spec
  - supporting: accepted ADR or durable background decision
  - nearby: RFC/proposal if subject match is strong
  - exclude body by default: protocol, template, model

- `implementation_or_resume`
  - primary: active plan, OpenSpec proposal/design/tasks, implementation spec
  - supporting: relevant ADR/RFC/design
  - nearby: protocol only if explicitly requested

- `protocol_or_instruction`
  - primary: root/repo instruction or named protocol/skill
  - supporting: module instruction only when query names module
  - nearby: plans/specs by subject only

Budgets are soft. Exact path/title/identifier matches can override them.

## Default Policy

Keep routing simple:

- use query words and phrases
- use artifact family terms
- use explicit lane terms such as instruction, template, schema, OpenAPI
- avoid repo-specific terms
- when uncertain, use broad_context and tier candidates conservatively

## Eval Integration Lift

Medium.

File-level and tier-level eval can measure routing impact without new labels.

Add diagnostics:

- chosen query mode
- confidence
- lane budget applied
- budget overrides

Optional future labels:

- expected query mode per eval case
- primary/supporting/nearby expected tier

Do not require those labels for the first pass.

## Auditable Success Criteria

- Every retrieval result records chosen query mode and lane budget.
- Canonical and real dev50 must-have recall do not regress.
- Real dev50 primary precision improves once tiered output is available.
- Protocol/template/model docs do not enter primary context unless query mode allows them or exact match overrides.
- Unit tests cover each query mode with generic sample artifacts.
- No implementation rule contains repository names or mined-case-specific phrases.

## Rollback Criteria

- Query mode misclassification causes must-have artifacts to be hidden from surfaced tiers.
- The router becomes opaque or hard to explain.
- Lane budgets prevent sparse repos from returning the only plausible artifact.
