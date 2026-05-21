# Artifact Lanes And Hierarchy Plan

Date: 2026-05-21

## Discussion Goal

Model how devspecs should index repository context beyond isolated markdown files, especially OpenSpec framework directories, while keeping the current north star focused on first-index precision for engineering intent artifacts.

This is a discussion basis, not an implementation commitment. The plan separates the minimum useful hierarchy work from broader future support for procedure and model context artifacts.

## Current State

The CLI already has an `openspec` adapter, but it is file-oriented:

- discovers `openspec/changes/<change>/proposal.md`
- discovers `openspec/changes/<change>/design.md`
- discovers `openspec/changes/<change>/tasks.md`
- discovers `openspec/changes/<change>/specs/<capability>/spec.md`
- stores each discovered file as its own artifact
- uses `sources.layout_group` to place companion files under the same change directory

This means the CLI has some grouping hints, but not first-class collection or bundle artifacts. The miner has more explicit preservation of structures, while devspecs-cli currently mostly stores files plus layout metadata.

Existing schema that can help:

- `artifacts.kind` and `artifacts.subtype`
- `sources.source_type`
- `sources.format_profile`
- `sources.layout_group`
- `links.link_type` and `links.target`
- `artifact_revisions.extracted_json`

Missing or weak model:

- no explicit `artifact_scope` field for `file`, `bundle`, or `collection`
- no explicit `lane` field for `intent`, `procedure`, or `model`
- no adapter API for returning relationship edges directly
- no retrieval rule that expands a parent bundle into child files or collapses child files into a parent bundle
- no first-index metric that distinguishes "found 1 OpenSpec change bundle" from "found 27 markdown files inside one OpenSpec tree"

## Terminology

Use these terms cautiously:

- **File artifact:** one source file that can stand alone as a context artifact.
- **Bundle artifact:** a coherent group of files under one local purpose, such as `openspec/changes/<change>/`.
- **Collection artifact:** a higher-level repository structure containing multiple related bundles and base specs, such as `openspec/`.
- **Child artifact:** an individual file or sub-document that belongs to a bundle or collection.

For OpenSpec, the change directory is a bundle. The full `openspec/` directory is a collection. A single `proposal.md` is a child artifact and may also be independently useful, but it should not be the only unit devspecs understands.

## Proposed Lanes

Lanes should be orthogonal to artifact kind. They describe retrieval and product intent, not just file format.

### Intent Lane

Core focus for the current public eval and token-savings story.

Examples:

- ADRs
- RFCs
- PRDs
- plans
- decisions
- design docs
- OpenSpec change bundles and capability specs
- agent planning or handoff docs

Default behavior:

- indexed
- included in first-index eval
- eligible for retrieval by default
- counted in precision and recall reporting

### Procedure Lane

Action recipes and operational guidance. Useful for agents, but not the same as engineering intent.

Examples:

- `SKILL.md`
- agent skills
- workflow docs
- command prompt docs
- runbooks
- task recipes
- how-to docs
- devguide procedures

Default behavior for now:

- classify and tag
- optionally index behind a separate source type or lane metadata
- exclude from the core intent precision/recall numbers
- exclude from default retrieval unless the query asks for process, commands, workflows, or how-to guidance

Why this matters:

- The dev-50 run showed `skills/`, `plugins/`, `.claude/skills`, `.codex/skills`, commands, and how-to docs can look structurally similar to plans.
- These are not always false positives. They are often valuable context, but they should not inflate the engineering-intent lane.

### Model Lane

Machine-readable or semi-machine-readable configuration, contracts, schemas, and manifests.

Examples:

- OpenAPI
- GraphQL schemas
- JSON Schema
- Terraform
- Kubernetes manifests
- Helm charts
- CI manifests
- protocol or capability schemas
- Morphe or similar modeled system descriptions

Default behavior for now:

- classify only when cheap and confident
- do not include in core intent eval
- do not retrieve by default unless the query asks for API shape, infra, schema, deployment, or config context
- leave deeper parsing for later adapters

## OpenSpec Hierarchy Target

OpenSpec should move from file-oriented indexing to hierarchy-aware indexing.

### Collection

Path:

```text
openspec/
```

Suggested artifact:

```yaml
lane: intent
artifact_scope: collection
kind: spec
subtype: openspec_collection
source_type: openspec
format_profile: openspec
source_identity: openspec|collection
```

Purpose:

- record that the repo uses OpenSpec
- summarize active changes, archived changes, and base capability specs
- provide retrieval entry point when the user asks about "the spec system" or "current specs"

### Change Bundle

Path:

```text
openspec/changes/<change-id>/
openspec/changes/archive/<change-id>/
```

Suggested artifact:

```yaml
lane: intent
artifact_scope: bundle
kind: spec
subtype: openspec_change_bundle
source_type: openspec
format_profile: openspec
source_identity: openspec/changes/<change-id>|bundle
layout_group: openspec/changes/<change-id>
```

Children:

```text
proposal.md
design.md
tasks.md
specs/<capability>/spec.md
```

Purpose:

- count one change as one bundle for eval and retrieval
- preserve child file roles
- allow retrieval to include the whole change bundle under a token budget
- avoid treating large OpenSpec repos as thousands of unrelated markdown artifacts

### Base Capability Spec

Path:

```text
openspec/specs/<capability>/spec.md
```

Suggested artifact:

```yaml
lane: intent
artifact_scope: file
kind: spec
subtype: openspec_capability_spec
source_type: openspec
format_profile: openspec
layout_group: openspec/specs/<capability>
```

Purpose:

- model stable system requirements separately from change proposals
- link change spec deltas back to the affected base capability where possible

## Relationship Model

Start schema-minimal, then migrate only if needed.

### Phase 1: Schema-Minimal Relationships

Use existing fields:

- `sources.layout_group` for shared local grouping
- `artifact_revisions.extracted_json` for `lane`, `artifact_scope`, `child_roles`, and `bundle_paths`
- `links` for `contains`, `contained_by`, `companion`, `updates`, and `supersedes` edges

This is enough to test retrieval behavior without adding tables.

### Phase 2: First-Class Hierarchy If Needed

Add fields or tables only after the schema-minimal version proves useful:

- `artifacts.lane`
- `artifacts.scope`
- `artifact_edges` or stricter `links` semantics
- `artifact_members` for ordered bundle children

Do not start here unless existing `links` and extracted metadata become painful.

## Adapter API Changes

The current adapter API returns:

```go
Parse(ctx, candidate) (Artifact, []Source, todoparse.ParseResult, error)
```

Hierarchy needs one of these options:

1. Keep the API and make bundle candidates return one synthetic artifact with multiple sources.
2. Extend parse results to include links/edges.
3. Add a post-scan relationship pass that groups artifacts by `source_type`, `format_profile`, and `layout_group`.

Recommended first step:

- keep the adapter API stable
- add OpenSpec bundle candidates that produce synthetic bundle artifacts
- keep child file artifacts for role-level retrieval only if they are not duplicated into generic markdown
- add a small post-scan pass to create relationship links between bundle and children

## Candidate Ownership Rules

Avoid duplicate indexing across adapters.

Rules:

- OpenSpec adapter owns `openspec/**`.
- Markdown experimental discovery should skip `openspec/**` once OpenSpec hierarchy is enabled.
- Procedure classifiers may own `SKILL.md`, `.claude/skills/**`, `.codex/skills/**`, `.agents/skills/**`, `plugins/**`, and command prompt docs.
- Intent markdown discovery may still use procedure-like files as weak evidence, but should not count them as intent unless content and path evidence strongly supports a real repo-local plan or decision.
- Model adapters should own their native files instead of relying on generic markdown.

## Expected Lift

Small lift: duplicate control and metadata taxonomy.

- Add lane/scope metadata in classifier output.
- Add markdown skip for `openspec/**`.
- Add negative or lane-routing signals for `fixtures`, `templates`, `skills`, `plugins`, and command prompt docs.
- Update dev-50 reporting to split by lane.
- Expected size: roughly 0.5-1.5 engineering days.

Medium lift: OpenSpec bundle artifacts using existing schema.

- Discover `openspec/` collection candidates.
- Discover `openspec/changes/<id>/` bundle candidates.
- Generate bundle bodies from proposal/design/tasks/spec deltas.
- Preserve child role metadata.
- Store bundle `Source` rows and `layout_group`.
- Keep or link child artifacts without double-counting.
- Update retrieval and eval to count bundles.
- Expected size: roughly 2-4 engineering days.

Medium-large lift: first-class relationship behavior.

- Add adapter output or scan post-pass for links.
- Add `contains` and `contained_by` relationships.
- Update `show`, `list`, and retrieval output to make bundle membership visible.
- Add context assembly that expands or collapses bundles under token budget.
- Expected size: roughly 3-6 engineering days depending on UI and eval depth.

Larger future lift: model adapters.

- OpenAPI, GraphQL, Terraform, Kubernetes, JSON Schema, and related formats each need format-aware parsing and chunking.
- These should be added incrementally after the intent lane is stable.
- Expected size: per adapter, not one monolithic project.

## Eval Implications

The current first-index story should stay focused on intent.

Add eval dimensions without expanding the headline scope too early:

- intent precision
- intent recall
- procedure false-positive leakage into intent
- model false-positive leakage into intent
- OpenSpec bundle recall
- OpenSpec child-role recall
- duplicate artifact pressure, especially file flood versus bundle count

For OpenSpec, count both:

- bundle-level success: did devspecs find the change bundle?
- child-role success: did devspecs preserve proposal/design/tasks/spec deltas?

Do not count every child file as a separate top-level win in the headline metric.

## Proposed Implementation Sequence

1. Add lane and scope metadata only.
2. Route obvious procedure and model candidates out of the intent lane.
3. Prevent markdown experimental discovery from indexing `openspec/**` as generic markdown.
4. Add OpenSpec change bundle artifacts using existing schema.
5. Add OpenSpec collection artifacts if bundle behavior proves useful.
6. Add relationship links between collections, bundles, and child docs.
7. Update retrieval to expand bundle children when a bundle is selected.
8. Re-run dev-50.
9. Patch only if dev-50 improves.
10. Run validation-50.
11. Touch lockbox only after validation is stable.

## Open Questions

- Should child OpenSpec files be visible in `ds list` by default, or hidden behind bundle expansion?
- Should procedure docs be indexed by default but excluded from default retrieval, or behind an explicit experiment flag?
- Should lane live as schema columns or stay in `extracted_json` until eval stabilizes?
- Should agent entrypoints be intent, procedure, or a separate `agent_context` lane?
- Should reusable skills/plugins be procedure docs even when their content contains implementation plans?
- How should token-savings reporting treat procedure and model bundles once they become retrievable?

## Decision Needed

For the next implementation pass, choose one of:

1. **Conservative:** only add lane/scope metadata and prevent OpenSpec markdown duplication.
2. **OpenSpec-first:** add OpenSpec change bundles and route procedure docs out of intent.
3. **Taxonomy-first:** implement intent/procedure/model lanes broadly, but keep hierarchy minimal.

Recommendation: choose OpenSpec-first. It directly addresses the biggest dev-50 artifact flood while improving a real high-value intent format.
