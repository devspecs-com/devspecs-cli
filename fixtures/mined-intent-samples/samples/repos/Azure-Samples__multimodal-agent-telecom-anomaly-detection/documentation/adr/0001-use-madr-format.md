# Use MADR Format for Architecture Decisions

- Status: accepted
- Deciders: [Team / Initial Maintainers]
- Date: 2025-10-06

Technical Story: Establish a lightweight, structured way to capture architectural and
technical decisions early in the project lifecycle.

## Context and Problem Statement

We need a consistent, quick-to-apply method for recording design and architecture
decisions (e.g., packaging layout, tooling, dependency strategy). Without structure,
rationale is lost, onboarding slows, and future refactors risk re-litigating prior
choices.

## Decision Drivers

- Improve traceability of why structural and tooling choices were made
- Keep documentation lean and easy to maintain
- Enable future contributors to propose changes with awareness of history
- Avoid heavyweight ADR tooling overhead

## Considered Options

- Use MADR (Markdown Any Decision Records) template
- Use plain free-form Markdown notes
- Use a more formal ADR framework (Joel Parker Henderson style)
- Use an issue tracker only (labels + comments)

## Decision Outcome

Chosen option: "Use MADR template", because it balances minimal ceremony with consistent
structure, is readable in plain Markdown, and integrates seamlessly with Git history.

### Positive Consequences

- Decisions remain discoverable under `documentation/adr/`
- Consistent headings allow grepping and automated indexing later
- Low friction encourages more complete record of rationale

### Negative Consequences

- Requires discipline to assign sequential numbers and keep index
- Slight upfront overhead versus ad-hoc notes

## Pros and Cons of the Options

### Use MADR (chosen)

- Good, because structured and well-known format
- Good, because easy diffing / reviewing in Git
- Good, because minimal tooling required
- Bad, because numbering needs manual maintenance

### Free-form Markdown

- Good, because zero constraints
- Bad, because inconsistent phrasing makes searching harder
- Bad, because rationale sections might be skipped

### Formal ADR framework

- Good, because exhaustive structure
- Bad, because heavier than needed for project size

### Issue tracker only

- Good, because no new files required
- Bad, because decision content fragments across comments
- Bad, because issue closure can bury rationale

## Links

- Template: [0000-madr-template](0000-madr-template.md)
- MADR repo: <https://github.com/adr/madr>
