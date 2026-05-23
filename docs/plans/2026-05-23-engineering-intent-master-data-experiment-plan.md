# Engineering Intent Master Data Experiment Plan

Date: 2026-05-23

## Goal

Explore whether DevSpecs can improve retrieval precision by building an automatic local "engineering intent master data" layer: a lightweight entity and relationship index derived from the repository's own artifacts.

This is not a manually curated insurance-style master data system. It is a deterministic, local, auditable concept map that helps retrieval answer:

- What engineering concepts does this repo talk about?
- Which artifacts are about the same concept?
- Which artifact is more authoritative or current for that concept?
- Which tests, comments, models, or procedures are supporting evidence rather than primary intent?

## Hypothesis

Keyword retrieval is too global. A query such as "billing webhook replay protection" should first resolve to a local concept neighborhood, then retrieve the strongest artifacts inside that neighborhood.

An automatically built engineering intent entity index may improve precision by:

- reducing broad keyword leakage across unrelated docs
- grouping synonyms and path/title variants
- boosting artifacts with multiple independent signals for the same concept
- connecting plans/specs/tests/comments/code paths without requiring explicit links
- making authority priors concept-aware instead of artifact-only

## Non-Goals

- No manual verification UI in v0.
- No LLM dependency for the first experiment.
- No global/cloud ontology.
- No claim that inferred entities are ground truth.
- No hard filtering by entity in the first experiment.
- No git-history or transcript mining in this pass.

## 1. This Is What We Index

Index local, repo-specific engineering-intent signals into an entity layer.

Primary artifact sources:

- ADRs and decision records
- PRDs and product requirements
- RFCs, proposals, enhancements, KEP/TEP/SEP-style docs
- OpenSpec bundles, proposal/design/tasks/spec delta/canonical spec files
- implementation plans and roadmaps
- agent plans and handoff notes
- BMAD stories and story-like implementation artifacts
- design docs and architecture docs

Supporting artifact sources:

- test-case artifacts
- high-signal code comments
- protocol/procedure docs
- model/declarative docs such as OpenAPI, schema, manifest, workflow definitions
- template docs when explicitly requested

Candidate text fields:

- file path segments
- artifact title
- headings
- frontmatter
- OpenSpec change id and capability names
- ADR/RFC/PRD numeric identifiers and titles
- task/checklist text
- test names and parent test contexts
- code comment text
- extracted links and source paths
- classifier subtype/mode/authority/lifecycle metadata

## 2. This Is What Is Semantically Included And How To Detect It

The entity layer should include only engineering-useful concepts, not every noun phrase.

Entity classes:

- `capability`: user-visible or system capability, such as "customer portal" or "webhook replay protection"
- `component`: implementation component, package, subsystem, service, adapter, worker, module
- `decision_topic`: architectural or product decision area
- `requirement_topic`: requirement cluster, acceptance area, constraint
- `workflow`: process or operational flow
- `api_contract`: endpoint, schema, event, message, external contract
- `test_behavior`: behavior under test, regression, edge case
- `project_artifact`: named artifact family, such as an OpenSpec change or RFC series

Detection signals:

- normalized title/headline phrases
- strong path phrases: `billing-webhook-hardening`, `auth/session`, `openspec/changes/add-customer-portal`
- repeated heading terms across artifacts
- OpenSpec capability/spec names
- RFC/ADR/PRD titles
- test names that contain domain terms
- code identifiers when mirrored in docs or tests
- explicit markdown links between artifacts
- classifier subtype plus domain terms

Exclusion signals:

- common documentation boilerplate
- generic headings alone: overview, introduction, usage, examples
- dependency/package names unless linked to a repo concept
- generated/reference/template/example paths unless query asks for them
- isolated one-off terms with no supporting artifact evidence

## 3. These Are The Relevant Primitives

Add small, inspectable primitives before any advanced graph work.

### Entity

Fields:

- stable local id
- canonical label
- normalized key
- class
- aliases
- confidence
- source count
- first/last observed path
- authority aggregate
- lifecycle aggregate
- top terms

### Mention

Fields:

- entity id
- artifact id/path
- source type: title, path, heading, body, test_name, comment, link, code_symbol
- text span or heading path when available
- confidence
- evidence weight

### Artifact Relation

Fields:

- source artifact
- target artifact/entity
- relation type
- evidence
- confidence

Initial relation types:

- `mentions`
- `same_concept`
- `part_of`
- `contains`
- `updates`
- `supersedes`
- `supports`
- `tests`
- `implements_hint`
- `references`

### Concept Neighborhood

A deterministic retrieval-time view:

- top entities for a query
- artifacts strongly attached to those entities
- supporting artifacts attached by tests/comments/models/protocols
- authority-sorted primary artifacts

## 4. These Are The Relevant Correlation Mechanisms

Use conservative correlation mechanisms first.

### Lexical Normalization

- lowercase
- split camelCase, snake_case, kebab-case, dotted identifiers
- singular/plural normalization for simple English suffixes
- remove stopwords and repo boilerplate terms
- preserve identifier phrases as aliases

### Phrase Extraction

Sources:

- file basename
- parent directory names
- H1/H2 headings
- OpenSpec change id and capability path
- ADR/RFC/PRD title
- test name
- code comment first sentence

Keep 2-5 token phrases with domain specificity.

### Alias Grouping

Group aliases when:

- normalized phrase keys match
- one phrase is a path-safe variant of another
- acronym is explicitly paired in title/body, such as `Product Requirements Document (PRD)`
- OpenSpec capability path and spec title align
- test name and doc title share a strong identifier phrase

Do not infer broad synonyms like "auth" equals "session" without repeated local evidence.

### Relationship Evidence

Boost relation confidence from:

- markdown links
- OpenSpec contains/updates links
- same directory family
- shared unique identifier terms
- doc title matches test name phrase
- code path appears inside plan/spec
- artifact references another artifact id/path

Downweight:

- generic term-only overlap
- archive/generated/example/template paths unless query intent matches
- stale/superseded artifacts unless query asks for history

### Authority Aggregation

Entity authority should aggregate from attached artifacts:

- accepted/current ADRs
- active OpenSpec changes
- canonical OpenSpec specs
- PRDs/RFCs with high classifier confidence
- recent/active plans
- tests as supporting behavioral evidence
- code comments as supporting rationale evidence

Authority should boost, not hide. Low-authority entities can still surface under exact query match.

## 5. This Is How We Can Use Them To Improve Results

Use the entity index as a retrieval assist, not a replacement.

### Query Entity Resolution

For each query:

- extract query phrases and identifiers
- resolve top N local entities
- emit `query_entities` diagnostics
- keep confidence and evidence visible

### Retrieval Boosts

Boost artifacts when:

- artifact is directly attached to a high-confidence query entity
- section heading matches a query entity
- artifact shares entity neighborhood with an exact path/title match
- tests/comments support an already selected intent artifact
- multiple independent artifact types mention the same entity

### Retrieval Filters

Use only soft filters initially:

- lower broad keyword matches outside the query entity neighborhood
- lower protocol/template/model artifacts unless query mode allows them
- lower comments/tests that are not tied to a resolved entity

### Context Packing

Entity-aware packing can:

- put primary intent artifacts first
- group supporting tests/comments below the entity they support
- avoid scattering unrelated artifacts that share generic words
- include short provenance: `entity: billing webhook replay protection`

### Eval Diagnostics

Add JSON diagnostics:

- resolved query entities
- entity confidence
- entity evidence sources
- artifacts included via entity boost
- artifacts suppressed due to weak entity relation
- per-case entity hit/miss summary

## 6. This Is How To Implement Against Status Quo

Stage 0: Plan-only and baseline

- Keep current retrieval unchanged.
- Add an offline command or eval-only builder that can emit entity index JSON for one fixture.
- Inspect entity quality manually on a few dev-tier repos.

Stage 1: Entity extraction

- Build entities from already indexed artifacts and metadata.
- Do not add a new scanner walk.
- Store the experiment output either in memory during eval or as JSON beside eval cache.
- Use deterministic sorting and stable ids.

Stage 2: Query resolution diagnostics

- Resolve query entities during eval.
- Add diagnostics only.
- No ranking changes yet.

Stage 3: Soft ranking boost

- Add small entity-neighborhood boosts in `internal/retrieval`.
- Keep recall guardrails.
- Add explain reasons such as `entity match: billing webhook replay protection`.

Stage 4: Supporting evidence routing

- Use entity links to include tests/comments as supporting context only when they reinforce selected primary intent artifacts or the query asks for behavior/rationale.

Stage 5: Promotion decision

- Compare dev-tier baseline vs entity experiment.
- If promising, run full real50.
- If full real50 improves precision or sufficiency without recall loss, keep as default or behind a conservative flag.

## Expected Data Structures

Initial JSON cache shape:

```json
{
  "schema_version": 1,
  "repo_snapshot": "...",
  "source_fingerprint": "...",
  "entities": [],
  "mentions": [],
  "relations": []
}
```

Potential future SQLite tables:

- `entities`
- `entity_aliases`
- `entity_mentions`
- `artifact_relations`

Do not add schema until eval shows retrieval value.

## Eval Strategy

Use cached dev-tier eval first.

Comparison:

- baseline current retrieval
- section-aware retrieval only
- entity diagnostics only
- entity-boosted retrieval
- section-aware + entity-boosted retrieval

Primary metrics:

- mean artifact precision
- graded precision
- mean artifact recall
- must-have recall
- context sufficiency pass rate
- must-hit@3
- mean first useful rank
- token reduction

Entity-specific diagnostics:

- query entity resolution rate
- entity-supported included artifacts
- false entity boost count from spot checks
- artifacts suppressed by entity locality

## Risks

- Entity extraction becomes noisy and reproduces keyword matching under a fancier name.
- Sparse repos have too little signal for useful entities.
- Alias grouping invents false synonyms.
- Entity boosts overfit to path naming conventions.
- Added diagnostics distract from core eval metrics.

Mitigations:

- soft boosts only
- no global synonym map in v0
- require multiple evidence sources for high-confidence entities
- no repo-specific rules
- keep dev-tier and full real50 promotion gates

## Auditable Success Criteria

- Entity extraction runs without additional repo walks by consuming indexed artifacts/candidates.
- Entity JSON output is deterministic across repeated runs with the same cache key.
- Query diagnostics list resolved entities, confidence, and evidence sources.
- At least 80% of dev-tier cases resolve either zero entities or plausible entities on manual spot check; zero entity is acceptable for generic queries.
- Entity-boosted retrieval improves mean artifact precision or graded precision on dev tier.
- Must-have recall does not regress by more than 2 percentage points on dev tier.
- Context sufficiency does not regress on dev tier.
- Entity boosts appear in artifact reasons when used.
- No rule contains repo names, mined-case-specific phrases, or manual-label-specific shortcuts.
- Full real50 is run before promotion to default behavior.

## Rollback Criteria

- Entity resolution introduces broad false boosts.
- Precision gains come mainly from hiding expected relevant artifacts.
- Entity diagnostics are too opaque to audit.
- Runtime overhead materially hurts the cached dev loop.
