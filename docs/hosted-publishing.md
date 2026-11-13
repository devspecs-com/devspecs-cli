# Experimental Hosted Publishing

`ds remote push` publishes one complete operator-approved subset of committed
records. It is not Git push, repository synchronization, indexing, or login. The
hosted service must explicitly enable records publishing and enroll its own
trusted Git source, approved refs, tenant/repository binding, and policy. Local
success alone does not demonstrate deployed service readiness.

## Explicit Invocation

Git Bash, from the selected local Git repository:

```bash
cd /d/path/to/approved-repository
ds remote push hosted \
  --commit "$(git rev-parse HEAD)" \
  --ref refs/heads/main \
  --confirm-url https://hub.example \
  --json
```

The commit must be a full lowercase 40- or 64-character Git OID reachable from
the named local `refs/heads/...` ref. Server enrollment must independently approve
that ref and source. `--confirm-url` must match the configured HTTPS base URL
exactly before vault access or network use. Remote add/list/remove remain local,
network-free and vault-free.

Interactive use requires a TTY and an existing session in the OS vault for the
exact origin/tenant/repository binding. Publishing never initiates device login,
upgrades capabilities, saves credentials, or falls back to plaintext. An ordinary
metadata-only login remains metadata-only until an operator explicitly grants
content publishing capability.

Noninteractive use requires the explicit environment pair; an absent, partial or
mismatched pair never falls back to the vault. Supply the token through the
environment, not command arguments or repository files. For example, Git Bash
can read an operator-provided token without echoing or placing it in history:

```bash
export DEVSPECS_TELEMETRY=0
export DEVSPECS_REMOTE_URL=https://hub.example
read -r -s -p 'Publishing token: ' DEVSPECS_REMOTE_TOKEN
printf '\n'
export DEVSPECS_REMOTE_TOKEN
ds remote push hosted --commit "$(git rev-parse HEAD)" \
  --ref refs/heads/main --confirm-url "$DEVSPECS_REMOTE_URL" --json
unset DEVSPECS_REMOTE_TOKEN
```

## Policy And Committed Records

The client first performs the bodyless authorized
`GET /v1/tenants/{tenant}/repos/{repo}/snapshots/policy`. The version-1 policy
contains only its schema/version, immutable policy ID, and labeled approved roots
of kind `task_parent`, `workspace_manifest`, `adr`, `rfc`, or `prd`. It provides no
filesystem location or trusted source identity. A publish-only credential can
discover policy and POST without gaining records-read permission.

Collection reads raw Git objects from the selected commit and its immediate
parents. It does not read worktree artifacts, untracked files, CRLF-converted
checkout bytes, local indexes/databases, or composed-document search identities.
Git runs with bounded output/deadlines and a minimal environment that excludes
inherited credentials and Git context. Hooks, fsmonitor, global/system config,
attributes, replacement objects, lazy fetching and network protocols are
disabled. Alternates, grafts, shallow and promisor sources are refused. There is
no fetch, checkout, Git push, checkpoint creation, ref/index write, or SQLite use.

Supported readers mirror the service's structured JSON/YAML/Markdown readers:
real CLI task manifests (including legacy manifests without `schema_version`),
schema-3 checkpoint JSON/Markdown pairs, schema-1 named threads, workspace/change
records and composed ADR/RFC/PRD documents. Logical task/workspace/change IDs are
normalized by the agreed readers; embedded paths remain content, not authority.
Unsupported schemas, unknown owner files, unsafe modes, ambiguity, incomplete
provenance or exceeded limits fail the entire snapshot, never a reduced subset.

For `workspace_manifest`, an optional operator-approved `artifact_root` must
exactly equal the resolved committed `artifact_dir` against the project prefix.
It is forbidden on other root kinds. When omitted, artifacts remain bounded
under the manifest's directory; repository YAML cannot expand authority to
`planning/`. Changing explicit policy scope requires a new immutable policy ID.
Workspace repository paths are not followed. Historical cross-policy root moves
and mutations hidden beyond immediate parents remain unsupported; the client
does not claim a complete unpublished-history audit.

Checkpoint immutability hashes fixed component roles (`event`, `markdown`) and
their raw bytes, not corpus locations, commit IDs or policy IDs. A legitimate
empty declared artifact is encoded as `content_base64:""` with the SHA-256 of
zero bytes; missing/null content is rejected.

## Wire And Limits

POST uses canonical `devspecs.hosted.snapshot` version 1 with `policy_id`,
`commit`, `ref`, and complete `records`. Each record contains kind, owner kind/ID,
record ID, schema version and files of exact path, raw lowercase SHA-256 and
standard base64 bytes. Canonical field order, record tuple order and file path
order match the service fixtures. No client-selected machine source path or
source identity is added to the envelope; raw record content can still contain
historical machine-path metadata.

Limits are 8 MiB encoded, 4 MiB decoded, 512 KiB per file, 1024 files, 512 records,
32 policy roots, 8 immediate parents and 16 path components. Git tree output is
also bounded to 8 MiB/32768 entries and a collection read has a 30-second deadline.
These limits fail whole, with no semantic fallback; they are not OS-level CPU or
memory isolation for arbitrary hostile repositories.

Policy discovery has a 5-second budget. POST has a separate 50-second budget
(45-second response-header budget), accommodating the service's bounded upload
and 40-second operation without changing metadata authentication budgets.
Redirects, proxy credential forwarding, compressed responses, automatic POST
replays and foreign result URLs are refused. Cancellation or a lost response
does not prove the POST failed: it may already have succeeded. The command never
automatically retries or alters the commit; resolve uncertainty with the operator
before explicitly retrying the same commit and ref with an unchanged policy ID
and definition. A policy change creates a new publication, not a retry. Policy is
discovered again on each invocation, so confirm it is unchanged with the operator;
do not blindly retry or change the source to clear an ambiguous outcome.

Successful POST returns version 1 and the exact configured-origin snapshot URL.
Human output is that URL; JSON output is `devspecs.remote.result` version 1 with
`status:"published"`, `remote`, `credential_source` and `url`, validated against
`specs/remote-result.v1.schema.json`. Existing auth/config result shapes are
unchanged. The client does not GET the URL: that requires separate records-read
authorization. Errors use the existing versioned error envelope and fixed safe
messages, never provider body text, credentials or raw Git diagnostics.

## Evidence And Remaining Gates

Public tests use real isolated Git repositories, TLS fixtures, canonical wire
fixtures and four independently generated service semantic goldens. Fixture
provenance is in `internal/hosted/snapshot/testdata/REAL-CLI-PROVENANCE.md`.
Synthetic golden commits are normalized solely for parity and are not reachable
publish requests. These tests do not establish live authentication, OS-vault
integration, customer-source enrollment or a deployed publish/read round trip.
Service packaging must contain Git and explicitly enable the records handler;
deployment, combined-suite verification and controlled live proof are separate
main-owned gates.
