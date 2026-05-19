# HexaKit -- Product Requirements Document

**Version:** 1.0 | **Status:** Active | **Date:** 2026-03-27

---

## Product Vision

HexaKit is a template and control-plane repository, not a product runtime. Its purpose is to keep
the shared shelf-level guidance, local documentation, and lightweight orchestration surfaces clean
and consistent across the Phenotype workspace.

HexaKit should stay minimal:

- No crate collection or heavy runtime code
- No dead catalog references
- No cross-repo governance drift
- No product-specific feature sprawl

## Scope

- Repository-level documentation
- Agent guidance
- Governance and status surfaces
- Lightweight support files that keep the template usable

## Success Criteria

- Root docs are concise and current
- Shelf guidance points to local truth surfaces
- Template-only expectations are preserved
- Merge debris does not reappear in live docs

## Out of Scope

- Application feature work
- Runtime service implementation
- Long-lived historical summaries in the repo root

## Notes

This PRD exists to define the repository role and keep it from accumulating product-specific
content that belongs in downstream projects.
