# ADR-0001: Keep composed documents repo-owned and index-independent

## Status

Accepted

## Context

DevSpecs has several forms of state with different lifetimes. Full task tracks
produce detailed slice receipts that may eventually be pruned. Workspaces route
work across repositories. The global SQLite index is local, derived, and
maintained by rebuild and prune operations. ADRs, RFCs, and PRDs need to survive
all of those operational lifecycles without ambiguous ownership or duplicate
copies.

## Decision

Composed documents are ordinary versioned Markdown owned by exactly one
repository and stored outside the DevSpecs task corpus. The repository file is
authoritative; index rows are derived and rebuildable.

Tasks may reference a composed document during the once-per-full-track durable
record closeout. Workspace callers route composition to the owning child with
`--repo`; DevSpecs does not add a parallel `ds workspace compose` surface or
copy a document into umbrella storage. `ds prune` changes only the local index,
never repository files. A rebuild rediscovers conventional or configured
document paths, and custom output directories must be configured when their
names do not express a supported convention.

## Consequences

- Positive: Durable decisions survive task pruning, database pruning, index
  rebuilds, and workspace reorganization.
- Positive: CLI and storage ownership stay small: compose is repo-local and
  workspace routing reuses `--repo`.
- Negative: Teams using unconventional document directories must configure
  discovery to guarantee deterministic rebuilds.
- Follow-up: Keep adapter ownership exclusive so one source file produces one
  canonical retrieval result.

<!-- devspecs: task=compose-product-integration target=E00 -->
