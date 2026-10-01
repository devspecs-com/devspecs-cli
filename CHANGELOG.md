# Changelog

## Unreleased

## v1.5.0 - 2026-10-01

- Added experimental repo-scoped `ds hub` for local coordination: discoverable
  topics, free-text messages, owner-managed JSON Schema event types, validated
  events, and durable filtered pull/ack subscriptions. The hub has its own
  SQLite authority and is not rebuilt from the source index or synchronized
  across machines. One replaceable actor vote per message revision supplies
  advisory up/down counts and optional ranked discovery without changing pull
  order. Listener processes and workload-enforcing locks are not included.
- Added opt-in `global:` hub topic and subscription addresses shared by
  unrelated repositories using the same DevSpecs home. Bare repository
  commands stay isolated; this is not a cross-machine service. Existing hub
  data migrates transactionally.
- Added experimental `ds hub lease acquire|show|renew|release` for atomic,
  time-bounded cooperative admission on repo or explicit `global:` topics.
  Holder tokens prevent stale renewal/release. Expired leases can be reclaimed,
  so participants must stop or renew before their deadline; DevSpecs does not
  contain jobs or block commands that skip the protocol. Hub schema v8
  migrates older local authorities on first write-capable open.
- Added explicit `ds prune --hub --before <RFC3339>` with dry-run, 10,000-entry
  batches, a verified pre-deletion backup, and replay-gap preservation.
  Default `ds prune` remains index-only. Optional `--vacuum` compacts the hub
  file and reports measured bytes reclaimed. Explicit cutoff pruning now
  includes old unpinned one-shot messages and complete old event correction
  chains. Large-hub cost, retained metadata growth, and groups exceeding the
  10,000-publication batch limit remain under review; this is not automatic
  garbage collection.
- Made `ds map` and `ds recent` fail with a bounded Git evidence error instead
  of silently omitting receipts after a timeout or cancellation. Existing map
  output caches are rebuilt so incomplete cached evidence is not reused.
- Kept the top-ranked map key paths intact while adding at most two
  complementary test anchors for concrete path boundaries when the ranked
  path budget would hide them. Broad conceptual parents keep the original cap.
- Hardened the opt-in fat-regression infrastructure: reject dirty corpus
  checkouts and empty, incomplete, duplicate or incompatible result sets;
  require preflight counts and observed successful command exits before
  reporting a passing aggregate gate. This does not enable a runner or promote
  changed output automatically.
- Added repository-owned Git Bash and PowerShell development launchers with
  deterministic named-channel or source-worktree homes under
  `~/.devspecs-dev`. Local builds and cross-repo agent sessions can now avoid
  the stable `~/.devspecs` database without adding public CLI surface or
  inferring behavior from executable paths.
- Changed activation evals, scan benchmarks, local pre-commit tests, and CI
  jobs to use isolated DevSpecs homes with telemetry disabled. Disposable
  benchmark homes no longer create production anonymous installation IDs.
- Added a provider-neutral `integrations.orchestration` repository setting and
  experimental `ds dispatch` workflow. Its first DevSpecs-owned driver uses
  stock Waspflow commands to freeze an exact `ds apply` target, monitor it to a
  normalized receipt by default, and support detached `status`, `resume`, and
  `receipt` flows without changing Waspflow or automatically promoting a task.
  Configuration alone never launches work, and release packages still ship one
  `ds` binary.
- Added read-only `ds doctor` diagnostics for active binary and PATH
  precedence, inferred install source, DevSpecs home, index size/schema/writer
  state, repository identity, and explicitly configured orchestration-provider
  readiness, with complete JSON output and structural `--redact` support for
  shareable reports.
- Added focused Linux, macOS, and Windows CI coverage for doctor executable
  discovery and read-only SQLite inspection.
- Added advanced `ds index backup|rebuild|restore` maintenance commands.
  Backups are schema-agnostic and WAL-consistent, rebuilds stage and validate a
  full cold index before publication, and restores preserve the displaced index
  while publishing the selected snapshot exactly.
- Changed supported index migrations to copy-on-write, backup-first upgrades.
  The active database is replaced only after the staged migration and rollback
  snapshot validate; interrupted recovery remains explicit and visible to
  `ds doctor`.
- Added real v1.3.0 schema-14 migration and rollback validation, synthetic
  WAL-backed future-schema refusal/backup/rebuild coverage, and three-platform
  file-swap recovery checks. The five-repository full-history activation corpus
  remains exact after recovery.
- Added file and stdin input for the free-text `ds task checkpoint` fields:
  `--description -` and `--note -` read all of stdin, `--description-file` and
  `--note-file` read a file. Each field takes exactly one source, only one flag
  may consume stdin per invocation, CRLF is normalised to LF, one trailing
  newline is trimmed, and interior newlines survive into the checkpoint JSON,
  the Markdown twin, and `--draft` previews.
  Multiline notes and descriptions remain nested under their Markdown field
  in result history and completion contracts.
- Pointed default telemetry at the canonical `www.devspecs.com` endpoint so each
  usage event avoids the apex-host redirect. The shell and PowerShell installers
  use the same endpoint. Command telemetry still uses a bounded synchronous
  request; this does not remove network waiting from command completion.
- Surfaced the existing checkpoint handoff flags where agents actually read
  them. The bounded `ds apply` slice prompt and the closeout prompt now name
  `--next-target`, `--next-decision`, `--missed-file`, `--noise-file`, and
  `--from-git`, with a validated-completion example using the real task and
  current target IDs without guessing the next lane. The `ds tldr handoff`
  workflow lists the checkpoint command that
  records the next target, and the generated Codex, Cursor, Claude, and
  Windsurf adapters name the same flags. Text only: no command, flag, or
  recorded field changed.
- Generated task slice and follow-up slice file names now cut their slug at a
  word boundary inside a 64-character budget instead of truncating mid-word at
  48 characters, so plan and result names no longer read like
  `...-with-environme-plan.md`. De-duplication and follow-up ordinal suffixes
  count against the budget. Slugs already recorded in a `task.json` manifest
  stay byte-identical; nothing on disk is renamed. Commands that take a slice
  selector also accept the slug a slice actually carries in its file name, in
  addition to every selector they accepted before.
- Split the `ds task audit` verdict so a mispredicted pack no longer reads as
  agent drift. Out-of-scope paths are classified as `pack_miss` (recorded with
  `--missed-file`, listed in the pack's own `noise_risks`, or tracked at git
  HEAD inside a predicted `relevant_areas` subsystem, or shared surface no
  subsystem owns such as a repository-root file or documentation),
  `new_surface` (not tracked at HEAD, so this slice created it), or `drift`
  (a tracked source file outside every relevant area). `drift` now means the
  slice reached a source subsystem the pack never identified as relevant,
  rather than any edit the pack failed to name, so routine `CHANGELOG.md` and
  docs updates no longer end every slice in a drift verdict.
  `recommendation` gained the `pack_miss` and `new_surface` values, and
  `--json` gained `pack_miss_paths`, `new_surface_paths`, `drift_paths`, and
  `unclassified_paths` alongside the unchanged `out_of_scope_paths` union.
  Neither fallback assigns blame: with no relevant areas recorded a tracked
  unpredicted path is a `pack_miss`, and when HEAD tracking cannot be
  determined the paths stay unclassified and the verdict falls to `review`.

## v1.4.0 - 2026-08-12

- Added experimental repo-first named execution threads with `ds thread set`,
  `ds thread remove`, owner-wide status, dependency joins, and explicit `ds
  apply <owner> --thread <key>` prompts. Tasks remain repo-owned by default;
  only tasks explicitly linked to a workspace change join its cross-repo graph.
- Hid `ds task next` from normal help while retaining callable compatibility
  for legacy linear tasks and one unambiguous lane. Multi-lane status and apply
  paths never choose the first runnable lane and instead print exact thread and
  apply commands.
- Made immutable checkpoint JSON events the durable lifecycle authority for
  named-thread scheduling, with YAML graph definitions and a rebuildable SQLite
  projection that reconstructs equivalent cold status after prune. No JSONL or
  remote service is required.
- Serialized thread graph mutations with repo and linked child-task checkpoint
  publication, preserving every event and preventing a workspace graph edit
  from overlooking newly started work. Blocked lanes now print an exact
  follow-up or conflict-resolution command.
- Preserved repo-owned durability closeout after a cross-repo graph completes:
  `ds apply <linked-task> --repo <child-repo>` returns that task's `A00`-style
  closeout, while applying the completed workspace change remains terminal.
- Added experimental `ds compose adr|rfc|prd` to create indexed, repo-owned
  durable drafts outside the DevSpecs task corpus while reusing established
  document directories, numbering, and ADR conventions. Composed files remain
  authoritative through index rebuild/prune operations, and workspace callers
  route ownership explicitly with `--repo` rather than a parallel workspace
  compose command. Conventional ADR paths retain single-adapter ownership after
  rebuild so retrieval does not return duplicate records for one file.
- Added ADR templates for all formats compared by adr.zone: Nygard, MADR full
  and minimal, Y-Statement, Outcome-First, and the ISO 42010 Companion.
- Added a one-time durability closeout after full task-track implementation
  slices finish. New full tracks must explicitly record that no durable document
  is needed, link completed ADR/RFC/PRD artifacts, or defer the record to a
  named target; compact `--quick` tasks and existing manifests keep their prior
  lifecycle behavior.

- Changed concurrent index mutations to queue behind one bounded writer lease so
  overlapping `scan`, `map`, `find`, `recent`, and `task` operations do not fail
  with transient SQLite lock errors.
- Added visible 10-minute auto-index and 30-minute explicit-scan deadlines, with
  signal cancellation that reaps active Git subprocesses instead of leaving
  long-running workers behind.
- Changed `ds task` creation to stage and validate complete workspaces before
  atomic publication. Interrupted preflight no longer leaves an empty final
  task directory, forced retries safely replace remnants, and failed success
  output now returns nonzero with the durable task ID and path in the error.
- Changed indexed task creation, slice addition, checkpoints, and legacy
  lifecycle mutations to verify local database compatibility before writing
  repository files. An older CLI against a newer derived index now reports the
  database, schema, executable, recovery, and that no repository files were
  written instead of leaving partial task state that retries can duplicate.
- Added `ds prune`, with `--dry-run`, JSON output, and explicit `--vacuum`
  compaction, to remove index data for repository roots that no longer exist
  and collapse redundant consecutive capture revisions without losing content
  transitions.
- Added bounded prune/vacuum progress on stderr for long human-mode maintenance
  waits while preserving clean JSON stdout.
- Changed Git repository identity from physical path alone to canonical remote
  plus root commit, with root aliases so ephemeral worktrees reuse one logical
  repository index instead of duplicating all artifacts.
- Scoped relative artifact source identities to their logical repository so
  common paths such as `AGENTS.md` cannot collide across unrelated repos.
- Added conservative repair for legacy cross-repository source ownership,
  including source-ownership indexes that keep fat-index prune checks bounded.
- Changed repeated `ds capture` calls so unchanged content refreshes metadata
  without appending another revision.
- Changed `ds update` detection so a manually installed `/usr/local/bin/ds`
  binary is not reported as Homebrew without stronger Homebrew path evidence.
- Changed top-level command failure rendering to print actionable errors once
  without an unrelated usage dump.
- Fixed `ds task refresh` after checkpointed work so lifecycle reconciliation
  preserves the newer capture timestamp and status does not immediately report
  the refreshed artifacts as stale again.

## v1.3.0 - 2026-07-12

- Added `ds task --quick` for compact one-off task workspaces and hid the older
  `ds task quick` form from normal help as compatibility surface.
- Changed `ds apply` with no target to resolve the unambiguous next slice and
  added next-target guidance to `ds task status`; help/docs now present
  argument-free `ds apply` as the happy path.
- Added `ds task slice add --after <slice> --reason <gate>` for A01-1-style
  follow-up slices and hid `ds task iteration` from normal help.
- Hid legacy task lifecycle shortcuts (`prompt`, `finish`, `decide`, `start`,
  and `sync`) from normal help while keeping compatibility paths callable and
  redirecting users toward `ds apply`, `ds task checkpoint`, and
  `ds task refresh`.

## v1.2.0 - 2026-07-12

DevSpecs v1.2 expands the CLI from repo-local task execution into workspace-aware
coordination, stronger regression discipline, better first-run map/recent
quality, and faster cold indexing.

Workspace and task coordination:

- Added experimental workspace coordination commands for umbrella repos:
  `ds workspace change create`, `ds workspace slice create`, and
  `ds workspace trace`. Top-level `ds change`, `ds slice`, and `ds trace`
  remain hidden compatibility aliases, with regression coverage for alias help
  and dispatch.
- Added `ds ws` as a built-in shortcut for `ds workspace` and introduced
  `specs/cli-surface.yaml` as the first canonical CLI surface audit artifact.
- Added explicit `--repo` routing for repo-local task and apply flows so agents
  can work from an umbrella root without writing task artifacts into the wrong
  repo.
- Added workspace trace output for known change/task IDs, including per-slice
  lifecycle status and aggregate workspace change completeness.
- Added `ds task checkpoint --draft` to preview checkpoint markdown, structured
  JSON evidence, and result append text without mutating task lifecycle state.
- Added `ds task checkpoint --from-git` to populate edited-file evidence from
  current git status/diff paths.
- Added `ds task checkpoint --run-log` to ingest explicit test/build/typecheck
  run logs as actual run evidence plus bounded structured output.
- Changed checkpoint result appends to convert the initial instruction section
  into `## Checkpoint History` after the first real checkpoint.

Quality and activation regression infrastructure:

- Added activation regression support for baseline-vs-candidate binary
  comparisons, canonical skinny/fat/full-history repo manifests, structured
  `ds map` comparison, and self-vs-self determinism checks.
- Added a cross-command cold activation gate for substrate consumers so
  `recent`, `map`, `find`, and `task quick` can be checked together before
  promoting indexing or output changes.
- Added a reviewed fat-100 activation quality baseline that layers strict
  v1.1.0 comparisons with accepted manual/automatic delta classifications,
  allowing future runs to distinguish true regressions from already-reviewed
  same-or-better output changes.
- Archived and documented canonical regression sets so future performance and
  quality work can start from the same small, fat-25, and fat-100 repo sets
  instead of rediscovering old manifests.

Map and recent quality:

- Improved `ds map` first-run behavior so default `map` builds the required
  local substrate before producing handoff commands instead of returning weaker
  index-missing output.
- Improved `ds map` boundary ranking and handoff suggestions with indexed
  packability checks, source/test balance signals, cached map output, and
  stricter structured comparison gates.
- Improved `ds recent` topic quality for noisy public repos such as FastAPI by
  merging overlapping recent work, demoting generic maintenance/setup topics,
  preserving specific README/spec/version-manifest topics, and surfacing
  system-boundary hints when available.

Cold indexing performance and UX:

- Added a fresh-index writer path for empty/cold repositories with batched row
  writes, deferred FTS updates, transaction-aware scans, and avoided per-file
  authored-at lookups where first-run semantics are unchanged.
- Reduced cold scan cost in source manifest, test-case, source companion, and
  evidence graph paths, including parallel source/test discovery and lower
  evidence mention construction overhead.
- Added phase timing and benchmark output for cold first-index runs, including
  source manifest, evidence graph, DB/write, and cross-command activation
  telemetry.
- Improved non-quiet cold-start progress output for `recent`, `map`, `find`,
  `task`, and scan-backed auto-indexing. Default progress now reports
  high-level checkpoints on stderr, while `--verbose` exposes detailed
  discovery, extraction, persistence, evidence graph, source manifest, and
  search-index phases without polluting result stdout or JSON output.

CLI surface polish:

- Clarified `ds scan` help copy so it describes a repository intent/source/test
  rescan instead of only specs, plans, and ADRs.

## v1.1.0 - 2026-06-17

DevSpecs v1.1 centers the launch story on bounded task execution for AI coding
agents.

- `ds task` is the primary workflow for creating packed, repo-grounded task
  slices with plan/result artifacts, checkpoints, and decision gates.
- `ds apply` emits the next bounded one-slice agent prompt without mutating task
  state.
- `ds init` can generate Codex, Cursor, Claude, and Windsurf adapter files for
  `ds task` and `ds apply`.
- `ds tldr` provides LLM-oriented quickstarts grouped by setup, hotfix, epic,
  incident, brownfield recovery, handoff, and repo deep dive workflows.
- `ds find` now builds packed context by default; use `ds find --plain` for the
  older flat result list.
- `ds map` focuses on architecture/system boundaries, while `ds recent` covers
  recently active local git topics.
- `ds update` reports the active binary, likely install source, latest release
  status, and recommended update command.

Known launch caveats:

- `ds apply` is prompt-only in v1.1; it does not launch an external coding
  agent.
- Generated agent adapter files are thin wrappers over the local CLI, not a
  hosted service.
- `ds adopt` is planned, not included in v1.1.0.
- Workspace coordination is experimental dogfood surface and may be renamed or
  consolidated before public launch.
