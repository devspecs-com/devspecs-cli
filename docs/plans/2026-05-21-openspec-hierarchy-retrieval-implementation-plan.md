# OpenSpec Hierarchy And Retrieval Implementation Plan

Date: 2026-05-21

## Goal

Implement OpenSpec-first hierarchy support in devspecs-cli with first-class parent/child links and retrieval expansion.

The immediate objective is to stop treating OpenSpec repositories as floods of unrelated markdown files. A repo with many `openspec/changes/<change-id>/...` files should index coherent change bundles, preserve child file roles, and retrieve the right proposal/design/tasks/spec deltas together under a token budget.

This plan targets the current north star: higher first-index precision and credible token-savings evals for engineering intent artifacts.

## Implementation Status

Code implemented in this pass:

- OpenSpec collection, change bundle, child file, and base capability spec artifacts.
- Deterministic bundle bodies with bundle-owned aggregate todos.
- OpenSpec parent/child/companion/update link post-pass.
- Generic markdown guardrail for `openspec/**`.
- Link-aware retrieval expansion from child files to parent bundles and from bundles to relevant children.
- Regression coverage for adapter discovery/parsing, scan links, markdown ownership, retrieval expansion, and command candidate metadata.

Still pending after this pass:

- Eval metric updates for `openspec.bundle_recall`, `openspec.child_role_recall`, `openspec.duplicate_pressure`, and `openspec.markdown_leakage`.
- Dev-50 and validation-50 reruns against the new hierarchy behavior.

## Definitions

- **OpenSpec collection:** the repository-level `openspec/` directory.
- **OpenSpec change bundle:** one `openspec/changes/<change-id>/` or `openspec/changes/archive/<change-id>/` directory.
- **OpenSpec child artifact:** a file inside a change bundle, such as `proposal.md`, `design.md`, `tasks.md`, or `specs/<capability>/spec.md`.
- **OpenSpec capability spec:** a base spec at `openspec/specs/<capability>/spec.md`.

## Current CLI Gap

The current OpenSpec adapter discovers child files and stores them as individual artifacts. It sets `format_profile=openspec` and `layout_group`, which is useful, but not enough:

- no parent bundle artifact exists for `openspec/changes/<change-id>/`
- no collection artifact exists for `openspec/`
- no durable parent/child links exist
- retrieval cannot expand from a matching child to sibling files
- eval can overcount one OpenSpec repo as hundreds or thousands of top-level wins
- experimental markdown discovery can still compete with OpenSpec ownership unless explicitly skipped

## Target Model

### Collection Artifact

```yaml
kind: spec
subtype: openspec_collection
source_type: openspec
format_profile: openspec
artifact_scope: collection
mode: intent
role: authoritative
source_identity: openspec|collection
```

Body:

- summary of active changes
- summary of archived changes
- summary of base capability specs
- pointers to child bundle/source paths

### Change Bundle Artifact

```yaml
kind: spec
subtype: openspec_change_bundle
source_type: openspec
format_profile: openspec
artifact_scope: bundle
mode: intent
role: authoritative
source_identity: openspec/changes/<change-id>|bundle
layout_group: openspec/changes/<change-id>
```

Body:

- normalized bundle header
- proposal excerpt or full proposal
- design excerpt or full design
- tasks excerpt or full tasks
- spec delta summaries grouped by capability

### Child File Artifact

```yaml
kind: spec | plan | design
subtype: openspec_child
source_type: openspec
format_profile: openspec
artifact_scope: file
mode: intent
role: authoritative
openspec_role: proposal | design | tasks | spec_delta
source_identity: <path>|openspec
layout_group: openspec/changes/<change-id>
```

Child artifacts remain useful for fine-grained retrieval, todos, criteria, and child-role eval. They should not be counted as independent top-level OpenSpec wins in the headline first-index metric.

### Base Capability Spec Artifact

```yaml
kind: spec
subtype: openspec_capability_spec
source_type: openspec
format_profile: openspec
artifact_scope: file
mode: intent
role: authoritative
openspec_role: capability_spec
source_identity: openspec/specs/<capability>/spec.md|openspec
layout_group: openspec/specs/<capability>
```

## Link Model

Use the existing `links` table first. Do not add schema unless this becomes painful.

Link types:

- `contains`: parent artifact contains child artifact
- `contained_by`: child artifact belongs to parent artifact
- `openspec_companion`: child artifacts are siblings in the same change bundle
- `updates`: change spec delta updates a base capability spec when the capability path is known
- `archived_from`: archived bundle supersedes or archives an active change identity if detectable

Target convention:

```text
artifact:<artifact_id>
source:<source_identity>
path:<repo-relative-path>
```

Implementation preference:

- use `artifact:<artifact_id>` when both artifacts are present in the local DB
- use `source:<source_identity>` when creating deferred links before all IDs are known
- use `path:<path>` only for external or missing children

## Implementation Steps

### Step 1: OpenSpec Adapter Ownership

Update OpenSpec discovery so it owns all `openspec/**` intent candidates.

Tasks:

- discover `openspec/` collection candidate when present
- discover one bundle candidate per `openspec/changes/<change-id>/`
- discover one bundle candidate per `openspec/changes/archive/<change-id>/`
- continue discovering child files for role-level artifacts
- discover base capability specs under `openspec/specs/<capability>/spec.md`
- mark OpenSpec candidates with `FormatProfile=openspec`
- mark OpenSpec candidates with `LayoutGroup`
- add metadata in `Artifact.Extracted` for `mode`, `role`, `artifact_scope`, `openspec_role`, `openspec_change_id`, `openspec_capability`

Auditable criteria:

- A fixture repo with one active change, one archived change, and one base spec produces:
  - `1` `openspec_collection`
  - `2` `openspec_change_bundle`
  - child artifacts for proposal/design/tasks/spec deltas
  - `1` `openspec_capability_spec`
- `ds list --json --source-type openspec` shows subtype and source path for each artifact.
- No `openspec/**` files are indexed through `source_type=markdown`.

### Step 2: Synthetic Bundle Body Generation

Generate deterministic bundle bodies for change bundle artifacts.

Bundle body format:

```markdown
# OpenSpec Change: <change-id>

## Bundle Metadata

- Status: <active|archived|unknown>
- Change Path: <path>
- Proposal: <path or missing>
- Design: <path or missing>
- Tasks: <path or missing>
- Spec Deltas:
  - <capability>: <path>

## Proposal

...

## Design

...

## Tasks

...

## Spec Deltas

### <capability>

...
```

Tasks:

- read child files in stable role order
- preserve original headings where practical
- add missing-child metadata without failing the scan
- hash synthetic body deterministically
- extract todos and criteria from `tasks.md` into the bundle artifact as well as the child artifact, or decide one canonical owner and document it

Auditable criteria:

- Bundle body is byte-stable across repeated scans with unchanged files.
- Changing `tasks.md` updates the bundle revision hash.
- Missing `design.md` does not fail scanning and is reflected in extracted metadata.
- Bundle todos are present for a query scoped to the bundle.

### Step 3: Relationship Post-Pass

Add a scan post-pass after all OpenSpec artifacts have been upserted.

Tasks:

- query OpenSpec artifacts by `repo_id`, `format_profile`, and `layout_group`
- link collection -> bundle using `contains`
- link bundle -> child files using `contains`
- link child files -> bundle using `contained_by`
- link child siblings using `openspec_companion`
- link spec deltas to base capability specs with `updates` when capability names match
- make link insertion idempotent

Auditable criteria:

- Re-running scan does not duplicate links.
- `ds show <bundle> --json` includes child links.
- `ds show <child> --json` includes `contained_by`.
- SQLite assertion passes:

```sql
SELECT COUNT(*)
FROM links
WHERE link_type = 'contains'
  AND target LIKE 'artifact:%';
```

### Step 4: Markdown Discovery Guardrail

Prevent generic markdown discovery from duplicating OpenSpec-owned files.

Tasks:

- skip `openspec/**` in experimental markdown intent discovery
- preserve configured explicit markdown paths if a user deliberately configures one, but mark duplication risk in metadata
- add regression tests covering `openspec/changes/foo/proposal.md`

Auditable criteria:

- In a fixture repo, `ds scan --experimental-intent-discovery` indexes OpenSpec files only through `source_type=openspec`.
- Dev-50 rerun shows OpenSpec no longer inflates `markdown` source counts.

### Step 5: Retrieval Expansion

Teach retrieval to use OpenSpec links.

Behavior:

- If a bundle scores highly, retrieve the bundle body first.
- If a child scores highly, add its parent bundle as a companion candidate.
- If a query asks for tasks, prioritize `tasks.md` child plus proposal context.
- If a query asks for design, prioritize `design.md` child plus proposal context.
- If a query asks for requirements or SHALL scenarios, prioritize spec deltas and base capability specs.
- Keep expansion under token budget.

Tasks:

- add link-aware expansion in retrieval candidate assembly
- add reason strings such as `openspec_parent_bundle`, `openspec_child_role:tasks`, `openspec_companion:proposal`
- add dedupe so bundle and child content do not both flood the final context
- expose expansion reasons in eval diagnostics

Auditable criteria:

- A query matching only a spec delta retrieves the parent bundle or proposal as context.
- A query matching only a task retrieves the proposal and task context.
- Retrieval reasons explicitly name the OpenSpec expansion rule.
- Token budget is respected in tests.

### Step 6: Eval Updates

Update first-index and retrieval evals so OpenSpec is counted correctly.

Tasks:

- count OpenSpec change bundles as bundle-level artifacts
- report child-role recall separately
- report duplicate pressure: file artifacts per bundle
- exclude child files from headline top-level OpenSpec precision unless the case explicitly evaluates child-role retrieval

Auditable criteria:

- Eval JSON includes:
  - `openspec.bundle_recall`
  - `openspec.child_role_recall`
  - `openspec.duplicate_pressure`
  - `openspec.markdown_leakage`
- Existing headline intent precision/recall can be reproduced before and after the change.

## Dev-50 Success Criteria

After implementation, rerun the same dev-50 set.

Required improvements:

- OpenSpec markdown leakage falls to `0` for `openspec/**`.
- `carverauto/serviceradar` reports OpenSpec bundles instead of thousands of top-level markdown wins.
- OpenSpec child roles are still discoverable.
- ADR and non-OpenSpec intent recall does not regress.
- No increase in procedure/template/fixture leakage into the intent lane.

Suggested acceptance thresholds:

- `openspec.markdown_leakage = 0`
- `openspec.bundle_recall >= 0.95` on manually checked OpenSpec repos in dev-50
- `openspec.child_role_recall >= 0.90` for proposal/design/tasks/spec-delta roles that exist
- headline intent precision improves versus current experimental dev-50
- headline intent recall does not drop by more than an explicitly reviewed amount

## Validation Criteria

Run validation-50 only after dev-50 improves.

Required validation checks:

- no repo-specific rules were added
- OpenSpec bundle behavior works on at least two unrelated OpenSpec repos, if present
- procedure/template/fixture leakage remains bounded
- retrieval expansion succeeds on at least one bundle query and one child-role query

## Rollback Criteria

Rollback or keep behind an experiment flag if:

- bundle artifacts make retrieval worse under token budget
- OpenSpec child roles become inaccessible
- bundle bodies create excessive token duplication
- dev-50 improves only because one repo was special-cased
- validation-50 shows lower precision with no compensating recall gain

## Dependency And Follow-Up Order

This OpenSpec plan should land before the broader mode/role taxonomy work because OpenSpec gives us a concrete hierarchy case to validate first: collection, bundle, child files, parent/child links, and retrieval expansion.

Follow-up taxonomy and adapter architecture plan:

- [2026-05-21-artifact-mode-role-adapter-architecture-followup-plan.md](2026-05-21-artifact-mode-role-adapter-architecture-followup-plan.md)

OpenSpec order:

1. OpenSpec collection/bundle/child artifacts.
2. OpenSpec parent/child links.
3. OpenSpec retrieval expansion.
4. Markdown `openspec/**` duplication guardrail.
5. Dev-50 rerun.
6. Validation-50 rerun.

Then mode/role architecture order:

1. Metadata-only mode/role classification.
2. Eval split by mode.
3. Procedure/template leakage controls.
4. Model adapter spikes.
5. Trace mode planning.
