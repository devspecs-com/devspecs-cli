# specs.place Bridge Plan

Date: 2026-05-21

## Goal

Make DevSpecs the local-first CLI bridge for specs.place corpora and resolver URIs.

Specs.place should remain the public/private registry and corpus hub. DevSpecs should make registry artifacts useful locally through import, indexing, search, context export, resolver lookup, and eventually private push/sync.

The first useful promise should stay small:

```text
ds corpus add specs.place/real-adrs-v0
ds find --corpus real-adrs-v0 "node reputation qos"
ds context spec://specs.place/100monkeys-ai/monkey-troop/docs/adrs/0016-node-reputation-and-qos.md
```

## Inputs

Original company-os direction:

- specs.place owns canonical public artifact pages, corpus pages/manifests, resolver URL shape, provenance display, and submission workflow.
- DevSpecs owns local scanning/indexing, corpus ingestion UX, local short IDs, context export, retrieval evals, and adapters.
- `spec://` should target specs.place-native artifact records, not act as a wrapped GitHub URL.
- V0 did not require bidirectional sync or private repo indexing, but the current product direction should preserve room for both.

Current specs.place registry shape:

- `GET /api/v1/corpora`
- `GET /api/v1/corpora/{slug}`
- `GET /api/v1/corpora/{slug}/manifest.json`
- `GET /api/v1/corpora/{slug}/manifest.jsonl`
- `GET /api/v1/artifacts/{id}`
- `GET /api/v1/resolve?uri=spec://...`

Current manifest records include registry ID, resolver URI, title, summary, source provenance, content hash, representation, classification, conformance, parse sections, validation, storage mode, corpus slugs, and links.

Important gap: the JSON/JSONL manifest currently omits `content`, even when `storage.mode == "full_text"`. DevSpecs can import metadata, summaries, and parsed section excerpts immediately, but full context export needs either a content-bearing manifest/export or per-artifact hydration.

## Product Stance

Public, no-login path:

- Public corpora should import without auth.
- Import should not clone source repositories or call GitHub.
- A small corpus should be searchable locally in under two minutes from a fresh DevSpecs install.
- DevSpecs should preserve license/storage posture in output, especially when content is metadata-only or excerpt-only.

Private path:

- Private push/sync should wait until specs.place has scoped registry repositories, auth, write APIs, and a clear storage/review policy.
- DevSpecs should design its local model now so private registry identity can be added without reworking search/context later.

## Terminology

Use separate nouns for separate jobs:

| Term | Meaning | Typical scope |
| --- | --- | --- |
| Local repo | A code project scanned by DevSpecs from a working tree. | One project/workspace. |
| specs.place registry repo | A registry-native repository or collection, owned by an org/user and addressable without requiring a backing GitHub repo. | One project, product surface, SSOT package, or internal spec set. |
| Corpus | A curated cross-repo dataset with explicit selection attributes, such as "Real ADR Corpus v0". | Many projects/sources, usually for browse, eval, examples, or training/evidence workflows. |

Corpora should be pulled through `ds corpus ...`. Private specs, Morphe definitions, OpenAPI definitions, and other SSOT records should eventually push/pull through registry repository commands, not corpus commands. A private corpus can still exist later, but it should be a dataset view over registry repositories or imported sources, not the default private repository analogue.

## Recommended CLI Shape

Prefer a `corpus` command group:

```bash
ds corpus add specs.place/real-adrs-v0
ds corpus add https://specs.place/api/v1/corpora/real-adrs-v0/manifest.jsonl
ds corpus list
ds corpus refresh real-adrs-v0
ds corpus remove real-adrs-v0
```

Then extend existing commands:

```bash
ds find --corpus real-adrs-v0 "auth token refresh decision"
ds list --corpus real-adrs-v0
ds show spec://specs.place/owner/repo/docs/adr/0001.md
ds resolve spec://specs.place/owner/repo/docs/adr/0001.md
ds context spec://specs.place/owner/repo/docs/adr/0001.md
```

`ds corpus add` is more idiomatic than adding a new root-level `ds add corpus` command. A hidden or documented compatibility alias can be added later if specs.place pages already use `ds add corpus`.

## Local Data Model

Short-term import can use the existing tables:

- `artifacts`: local DevSpecs row, still with a local `ds_...` ID.
- `artifact_revisions`: imported body or synthesized searchable body.
- `sources`: `source_type = "specs_place"`, `source_identity = registry artifact ID or resolver URI`, `path = resolver URI or source original path`.
- `links`: registry ID, resolver URI, specs.place web path, source URL, corpus slug.
- `artifact_tags`: domain and technical tags from specs.place classification.
- `artifacts_fts`: title, imported body, and resolver/source paths.

But the durable bridge should add explicit corpus metadata:

```text
corpora
  id, slug, registry, version, manifest_url, status, imported_at, refreshed_at

corpus_artifacts
  corpus_id, artifact_id, registry_artifact_id, resolver_uri, content_sha256,
  storage_mode, license, review_status

external_artifact_ids
  artifact_id, provider, external_id, resolver_uri, web_url
```

This avoids pretending public corpora are local git repos and gives `--corpus` a real filter instead of overloading `--repo` or tags.

Future private registry repository metadata should be separate from corpus metadata:

```text
registry_remotes
  id, provider, base_url, owner, name, visibility, default_ref, imported_at, refreshed_at

registry_remote_artifacts
  remote_id, artifact_id, registry_artifact_id, resolver_uri, ref, content_sha256,
  storage_mode, license, visibility, sync_state
```

This keeps "code project repo", "registry repo", and "cross-repo corpus" distinct while still letting all three feed the same local retrieval and context machinery.

## Milestone 0: Contract Hardening

Purpose:

Lock the specs.place import contract before CLI behavior depends on unstable fields.

DevSpecs work:

- Add an internal `specsplace` package with DTOs for corpus manifest JSON and JSONL.
- Add schema validation tests using the current `real-adrs-v0.catalog.json` / manifest shape.
- Decide the first import identity rule:
  - local artifact ID remains DevSpecs-owned,
  - specs.place artifact ID is stored as external identity,
  - resolver URI is lookupable directly.
- Decide content policy flags:
  - `--content=metadata` imports summary and parse excerpts only.
  - `--content=allowed` imports registry-provided full text when `storage.mode == "full_text"`.
  - no source-host fetching in the first milestone.

Registry dependency:

- Prefer adding `content` to the manifest when storage allows, or add a bulk content export. Per-artifact hydration can be acceptable for the seed ADR corpus, but it will not scale as the default.

Acceptance:

- DevSpecs can parse both manifest formats.
- Manifest field regressions fail tests with clear messages.
- A registry record can round-trip into an internal import struct without losing source, license, storage mode, resolver URI, or content hash.

## Milestone 1: Pull Corpus To Local Index

Purpose:

Make a public specs.place corpus searchable and context-exportable from DevSpecs.

CLI:

```bash
ds corpus add specs.place/real-adrs-v0
ds corpus list
ds find --corpus real-adrs-v0 "node reputation qos"
ds context spec://specs.place/100monkeys-ai/monkey-troop/docs/adrs/0016-node-reputation-and-qos.md
```

Implementation:

- Add `internal/specsplace/client.go` for registry base URL resolution and GET requests.
- Add `internal/specsplace/manifest.go` for JSON/JSONL parsing.
- Add `internal/specsplace/importer.go` to upsert corpus metadata and artifact rows.
- Add store methods for corpora, corpus memberships, external IDs, resolver lookup, and corpus filters.
- Extend `store.FilterParams` with `CorpusSlug`.
- Extend `ds list`, `ds find`, and retrieval candidate loading to include `--corpus`.
- Extend `ds show`, `ds resolve`, and `ds context` to accept `spec://...` and `sp:artifact:...` lookup keys.
- Preserve registry metadata in extracted JSON and output:
  - `specs_place_artifact_id`
  - `specs_place_resolver_uri`
  - `specs_place_web_url`
  - `source_url`
  - `license`
  - `storage_mode`
  - `content_sha256`
  - `corpus_slugs`

Default body strategy:

- If full text is present and allowed, index that.
- Otherwise synthesize a body from title, summary, parsed section excerpts, classification tags, source path, and resolver URI.
- Context export must clearly label synthesized/excerpt-only context so agents do not confuse it for complete source text.

Acceptance:

- Importing `real-adrs-v0` is idempotent.
- `ds corpus list --json` shows slug, version, artifact count, storage/content summary, and import time.
- `ds find --corpus real-adrs-v0 ...` retrieves imported registry artifacts without requiring cwd repo scope.
- `ds context spec://...` works after import.
- `ds show --json spec://...` includes both local DevSpecs ID and specs.place IDs.
- Public corpus import does not require login.

## Milestone 2: Resolver And Eval Bridge

Purpose:

Make specs.place artifacts first-class in retrieval demos, eval reports, and cross-tool citations.

Implementation:

- Add `ds resolve --remote spec://...` to call specs.place when an artifact is not imported.
- Add `ds corpus refresh` with ETag/Last-Modified support if the registry exposes it.
- Add `ds eval` support for a specs.place corpus as a corpus source.
- Add eval case import for specs.place `EvalCase` records once registry exposes them.
- Make context output cite:
  - DevSpecs local ID,
  - specs.place artifact ID,
  - resolver URI,
  - source URL,
  - corpus membership and license/storage posture.

Acceptance:

- Eval can run against imported specs.place artifacts.
- Eval output links failures/successes back to specs.place artifact pages.
- Remote resolver lookup can fetch metadata for a single `spec://` without adding the whole corpus.

## Milestone 3: Push Private Specs To specs.place Registry Repos

Purpose:

Let teams publish selected local DevSpecs artifacts into privately scoped specs.place registry repositories.

This milestone should start only after specs.place supports:

- auth tokens or device login,
- organizations/accounts,
- private scopes,
- write APIs,
- idempotent upload endpoints,
- visibility and permission checks,
- storage mode decisions,
- registry repository creation,
- audit trail,
- tombstones/delete policy,
- quota and abuse controls.

CLI:

```bash
ds auth login specs.place
ds registry create acme/platform-specs --private
ds push --to specs.place/acme/platform-specs --kind decision --subtype adr
ds push --to specs.place/acme/platform-specs --all --dry-run
```

Implementation:

- Add auth config under the DevSpecs home directory, separate from repo config.
- Add export packaging from local `artifacts`, `sources`, revisions, tags, todos, criteria, and links.
- Add dry-run diff before upload.
- Add upload idempotency keyed by local source identity, content hash, and target registry path.
- Add local link rows after successful push:
  - `specs_place_artifact_id`
  - `specs_place_resolver_uri`
  - `specs_place_web_url`
  - target private registry repo.

Acceptance:

- A local ADR can be pushed to a private specs.place registry repo and resolved back by URI.
- Re-running push with unchanged content is a no-op.
- Push output clearly distinguishes uploaded, unchanged, skipped, and rejected artifacts.
- Private resolver URIs do not leak through public corpus commands.

## Milestone 4: Bidirectional Sync

Purpose:

Make DevSpecs and specs.place cooperate over time without losing local-first ergonomics.

Implementation:

- Add `ds sync specs.place/acme/platform-specs`.
- Track last remote revision/content hash.
- Detect local-only, remote-only, unchanged, and diverged artifacts.
- Add conflict output before any overwrite.
- Support pull of private corpora into local indexes with the same search/context commands as public corpora.
- Support pull of private registry repos into local indexes without treating them as eval corpora.
- Add provenance edges that Kleio can consume later.

Acceptance:

- Sync never overwrites local content without explicit user action.
- Local search stays useful without network.
- Private corpora can be refreshed explicitly.
- Context output can identify stale imported records.

## Open Questions

- Should `spec://` resolution be owned semantically by specs.place, while DevSpecs implements local and remote resolver clients? Recommended: yes.
- Should public corpus artifacts appear in ordinary `ds find` by default? Recommended: no. Use explicit `--corpus` or `--include-corpora` so local repo context stays clean.
- Should DevSpecs use specs.place artifact IDs as primary local IDs? Recommended: no. Keep local `ds_...` IDs and store specs.place IDs as external identities.
- Should `ds corpus add` hydrate full content by default? Recommended: import registry-provided full text when storage allows, but do not chase source URLs.
- Should private push target corpora or repos? Recommended: repos. Corpora remain cross-repo datasets; registry repos are the private GitHub-like analogue.
- Should private push be milestone 2 or 3? Recommended: milestone 3, unless the registry write/auth layer lands before eval/resolver polish.

## First Engineering Slice

The smallest useful implementation slice in DevSpecs is:

1. Add specs.place manifest DTOs and tests.
2. Add a `ds corpus add <manifest-or-slug>` command that imports from a local fixture path first, then HTTP.
3. Store imported records with source type `specs_place`, links for registry IDs/URLs, and synthesized revision bodies.
4. Add `--corpus` to `ds find` and `ds list`.
5. Add resolver lookup for `spec://...` in `show`, `resolve`, and `context`.
6. Run an import/search/context golden test against the `100monkeys-ai/monkey-troop` ADR from `real-adrs-v0`.

This keeps the first bridge demonstrable without waiting on private registry infrastructure.
