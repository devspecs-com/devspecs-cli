# Proposal/RFC/Roadmap/Architecture Discovery Expansion

## Context

RFC and root roadmap support already exists in `devspecs-cli`, but coverage is still uneven for common co-located engineering-intent artifacts. Real repos often store these documents under proposal-family directory structures rather than as obvious `*.rfc.md` or root `ROADMAP.md` files.

The goal is to improve first-index coverage without turning discovery into a list of mined repo-specific paths.

## Scope

Expand generic markdown discovery and classification support for these general families:

- RFC/proposal/enhancement documents
- roadmap and milestone planning documents
- architecture and design documents
- proposal-family directory indexes such as `enhancements/<id>/README.md`

## Non-Goals

- Do not add repo-specific path names.
- Do not broaden discovery to all of `docs/`.
- Do not claim all proposal-like documents are RFCs without section/path evidence.
- Do not change OpenSpec hierarchy behavior in this pass.

## Plan

1. Add narrow default discovery paths for generalized engineering-intent conventions:
   - `proposals`, `docs/proposals`
   - `enhancements`, `docs/enhancements`
   - proposal acronym families such as `keps`, `teps`, `beps`, `sips`, `ships`, `oseps`
   - `architecture`, `docs/architecture`
   - `design-docs`, `docs/design-docs`
2. Teach broad intent scoring about proposal-family acronyms and architecture/roadmap strength.
3. Add section evidence for RFC/proposal and roadmap/architecture shapes:
   - summary, motivation, proposal, detailed design, drawbacks, unresolved questions
   - milestones, timeline, now/next/later
   - architecture, constraints, components, dependencies
4. Infer proposal-family paths as design artifacts so indexed candidates get a useful kind.
5. Extend classifier path hints with the same generalized families.
6. Add synthetic regression tests for discovery and classification, including negative controls.
7. Run focused unit tests and a small real dev-set scan/eval check.

## Auditable Success Criteria

- `beps/<id>/README.md`, `enhancements/<id>/README.md`, `docs/proposals/*.md`, `docs/design-docs/*.md`, and `docs/architecture/*.md` are discovered without custom config.
- Proposal-family directory index docs are inferred as `design`.
- Root and nested roadmap docs remain inferred as `plan`.
- Root `README.md`, changelogs, release notes, generated docs, and pull request templates are not admitted by broad discovery.
- Existing OpenSpec, ADR, PRD, Spec Kit, BMAD, agent-note, and configured markdown tests continue to pass.
- The implementation cites feature patterns in tests, not individual mined files.

## Validation Notes

The first-index eval should be rerun after this patch. If the broad path expansion increases noisy precision failures, revert the relevant family path and rely on scored discovery only for that family.
