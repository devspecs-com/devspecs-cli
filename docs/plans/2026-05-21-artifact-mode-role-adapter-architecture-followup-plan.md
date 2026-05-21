# Artifact Mode, Role, And Adapter Architecture Follow-Up Plan

Date: 2026-05-21

## Goal

Define the broader artifact taxonomy and adapter architecture direction that should follow the OpenSpec hierarchy work.

This plan depends on the OpenSpec hierarchy implementation plan:

- [2026-05-21-openspec-hierarchy-retrieval-implementation-plan.md](2026-05-21-openspec-hierarchy-retrieval-implementation-plan.md)

OpenSpec should land first because it gives us a concrete, high-signal hierarchy case: collection, bundle, child files, parent/child links, and retrieval expansion. This plan generalizes that model after we have measurable evidence from dev-50 and validation-50.

## Dependency And Order

Do first:

1. OpenSpec collection/bundle/child artifacts.
2. OpenSpec parent/child links.
3. OpenSpec retrieval expansion.
4. Markdown `openspec/**` duplication guardrail.
5. Dev-50 rerun.
6. Validation-50 rerun.

Then do this follow-up plan:

1. Metadata-only mode/role classification.
2. Eval split by mode.
3. Procedure/template leakage controls.
4. Model adapter spikes.
5. Trace mode planning.

Do not promote mode/role fields to schema columns until the metadata-only phase improves eval clarity and does not destabilize retrieval.

## Modes

Modes describe the kind of context represented by an artifact. They are orthogonal to artifact kind.

| Mode | Meaning | Examples | Default Initial Handling |
|---|---|---|---|
| `intent` | what/why | ADR, RFC, PRD, proposal, plan, design, rationale | index and retrieve by default |
| `protocol` | how/rules/constraints | procedure, skill, policy, standard, convention, checklist | classify, retrieve only when query asks |
| `model` | modeled state | schema, model, contract, configuration, manifest, workflow definition | classify, adapter-specific retrieval later |
| `template` | reusable form | document template, code scaffold, project scaffold, prompt template | classify, exclude from intent eval |
| `trace` | what happened | commit, pull request, issue thread, chat transcript, LLM transcript, log | future mode, not default indexed yet |

## Roles

Roles describe authority, provenance, and lifecycle. They are orthogonal to mode.

Initial role vocabulary:

- `authoritative`
- `canonical`
- `sample`
- `fixture`
- `generated`
- `inferred`
- `stale`
- `superseded`
- `template`
- `archived`
- `draft`
- `accepted`
- `rejected`

Examples:

```yaml
mode: intent
subtype: decision
role: authoritative
```

```yaml
mode: protocol
subtype: skill
role: generated
```

```yaml
mode: template
subtype: document-template
role: sample
```

```yaml
mode: model
subtype: manifest
role: canonical
```

```yaml
mode: trace
subtype: pull-request
role: authoritative
```

## Subtype Vocabulary

Intent:

- `decision`
- `requirement`
- `proposal`
- `plan`
- `design`
- `rationale`

Protocol:

- `procedure`
- `skill`
- `policy`
- `standard`
- `convention`
- `checklist`

Model:

- `schema`
- `model`
- `contract`
- `configuration`
- `manifest`
- `workflow-definition`

Template:

- `document-template`
- `code-scaffold`
- `project-scaffold`
- `prompt-template`

Trace:

- `commit`
- `pull-request`
- `issue-thread`
- `chat-transcript`
- `llm-transcript`
- `log`

## Adapter Architecture Direction

Adapters should evolve from "file type scanners" into "context artifact producers."

Target adapter responsibilities:

- own a path or format family
- emit candidates with mode/scope hints
- parse native structure
- emit child role metadata
- optionally emit links or deferred relationship intents
- avoid competing with generic markdown for owned paths

Suggested adapter families:

- `openspec`: intent bundles and capability specs
- `markdown`: generic intent files not owned by a specialized adapter
- `adr`: decision documents and ADR families
- `procedure`: skills, workflows, runbooks, command recipes, how-to docs
- `model`: schemas, manifests, contracts, configuration
- `template`: document templates, prompt templates, scaffolds
- `trace`: commits, PRs, issues, transcripts, logs

## Procedure Mode Notes

Procedure artifacts are valuable for agents, but they should not inflate the engineering-intent headline metric.

Candidate examples:

- `SKILL.md`
- `.claude/skills/**`
- `.codex/skills/**`
- `.agents/skills/**`
- workflow docs
- runbooks
- how-to docs
- command prompt docs
- task recipes

Initial behavior:

- classify as `mode=protocol`
- retrieve only when the query asks for process, workflow, runbook, command, or how-to context
- exclude from intent precision/recall headline metrics

## Template Mode Notes

Template artifacts are reusable forms. They often contain high-signal headings that look like intent documents, but they are not actual project intent instances.

Candidate examples:

- ADR templates
- PRD templates
- `.github/ISSUE_TEMPLATE/**`
- prompt templates
- code scaffolds
- project scaffolds

Initial behavior:

- classify as `mode=template`
- assign role `template` or `sample`
- exclude from intent precision/recall
- retrieve only when the query asks for a form, example, or template

## Model Mode Notes

Model artifacts represent modeled state, contracts, or configurations.

Candidate examples:

- OpenAPI
- GraphQL schemas
- JSON Schema
- Terraform
- Kubernetes manifests
- Helm charts
- CI manifests
- workflow definitions
- Morphe or similar modeled system descriptions

Initial behavior:

- classify cheaply and conservatively
- avoid default retrieval unless the query asks for API shape, infra, deployment, schema, or config context
- add deeper format-aware adapters incrementally

## Trace Mode Notes

Trace artifacts describe what happened.

Candidate examples:

- commits
- pull requests
- issue threads
- chat transcripts
- LLM transcripts
- logs

Initial behavior:

- keep trace as a planned mode
- do not index git history or transcripts by default yet
- require explicit user opt-in and provenance controls before implementation

## Eval Implications

Eval reports should split counts by mode:

- intent precision and recall
- protocol leakage into intent
- model leakage into intent
- template leakage into intent
- trace disabled or explicitly opt-in

Headline public metrics should continue to use intent until procedure/model/trace retrieval has product-ready behavior.

## Implementation Sequence

1. Add `mode`, `role`, `artifact_scope`, and `subtype` metadata in extracted JSON.
2. Teach eval reports to split intent/protocol/model/template/trace.
3. Route obvious templates and fixtures out of intent.
4. Route skills, workflows, runbooks, and command docs into protocol.
5. Add model classification only for cheap/high-confidence formats.
6. Add trace as a planned mode, but do not index git history or transcripts by default yet.
7. Promote stable metadata fields to schema columns only after eval proves the taxonomy useful.

## Auditable Success Criteria

- Eval JSON reports artifact counts by mode.
- Intent headline metrics are reproducible with non-intent modes excluded.
- At least one dev-50 rerun shows reduced template/procedure leakage into intent.
- Procedure artifacts remain discoverable by explicit query.
- Template artifacts are not counted as real ADR/PRD/RFC/plan instances.
- Model artifacts are not pulled into default intent retrieval unless the query asks for schema/config/contract context.
- No trace sources are indexed by default.
