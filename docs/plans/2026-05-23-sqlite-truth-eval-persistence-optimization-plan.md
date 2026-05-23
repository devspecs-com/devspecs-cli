# SQLite-Truth Eval Persistence Optimization Plan

Date: 2026-05-23

## Goal

Speed up uncapped real-repo eval indexing while keeping `ds eval` aligned with the persisted SQLite artifact model used by the real CLI.

## Non-Goal

Do not introduce an in-memory eval corpus as the canonical path. Fast dev approximations may be considered later only with explicit flags and parity tests, but this change keeps the temp SQLite index as the eval source of truth.

## Constraints

- Another agent is working on the Section-Aware Retrieval Plan. Avoid retrieval ranking, section extraction, and schema work in this patch.
- Default production `ds scan` behavior must remain compatible.
- Eval results should still be produced by scanning into a SQLite database and reading artifacts back from that database.

## Design

1. Keep the shared file inventory and progress heartbeats from the cold-scan optimization.
2. Add an eval-only fresh-index write mode for the temporary eval DB.
3. In fresh-index mode, skip existence/update checks that are impossible in a newly created empty DB, but still insert the same artifact, revision, source, todo, criteria, tag, and FTS rows.
4. Remove avoidable per-artifact tag lookup queries by deriving directory tags from already available source rows.
5. Preserve the normal update/upsert path for production `ds scan`.
6. Validate by comparing indexed artifact counts/shapes and by running uncapped progress probes on a slow repo.

## Success Criteria

- `ds eval` continues to use a temp SQLite DB as its indexed corpus source.
- Production `Scanner.Run` still exercises the ordinary upsert/update path.
- Eval `Scanner.RunWithOptions(... FreshIndex: true ...)` avoids impossible per-artifact existence checks on a fresh DB.
- Artifact rows remain available through existing `ListArtifacts`, `GetSourcesForArtifact`, `GetRevision`, and related store APIs.
- Uncapped progress probes show a higher parse/upsert throughput than the previous ~4,881 test artifacts in 131s on `apache__camel`.
- Focused scan/eval/command tests pass.

## Rollback Criteria

- Fresh-index mode changes artifact identity, body rendering, subtype metadata, or retrieval-visible paths compared with the ordinary SQLite path.
- Any production scan test fails or shows changed update behavior.
- Eval cache keys or JSON shape change beyond additive progress fields.
