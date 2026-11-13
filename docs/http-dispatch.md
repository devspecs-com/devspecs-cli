# Experimental Normal-Build HTTP Dispatch

This is controlled, trusted-provider functionality, not customer-general agent
readiness. The normal driver supports explicit inspection
goals and verified edited-result transfer. Public/private fixture parity and
live edited synthetic proof remain separate review gates. The tagged pilots
and their strict draft-2 wire schema are unchanged.

## Explicit Selection

Keep the existing native Waspflow configuration for native execution. HTTP is
never selected automatically. In a clean, committed execution repository:

```yaml
integrations:
  orchestration:
    provider: http
    options:
      goal: read_only
```

For an implementation goal, select `goal: edited` instead. No other repository
option is accepted. Repositories cannot select a profile,
endpoint, token environment name, TLS anchor, or caller identity.

The caller sets `DEVSPECS_HTTP_DISPATCH_CONFIG` to an absolute, explicitly chosen
JSON profile outside both execution and task repositories. Example shape:

```json
{
  "schema": "devspecs.dispatch.http_config",
  "schema_version": 1,
  "enabled": true,
  "read_only": true,
  "endpoint": "https://worker.example/v2",
  "source_url": "https://git.example/project/repo.git",
  "source_commit": "0123456789abcdef0123456789abcdef01234567",
  "principal_id": "reviewer",
  "credential_id": "reviewer-key-1",
  "token_env": "MY_DISPATCH_TOKEN"
}
```

The endpoint must be HTTPS. `source_url` is exact, unsigned HTTPS, not a
normalized approximation. `source_commit` is a complete immutable commit OID.
An optional absolute `ca_file` selects PEM trust instead of system roots; that
file must also be outside both repositories. Unknown/duplicate fields, disabled
profiles, missing explicit mode consent, invalid identities, and relative paths
fail closed. Existing paths and ancestors are resolved, with file-identity
comparisons for symlink/junction/short-name aliases and cross-volume paths.
The caller must protect the profile, CA, environment, state home, and checkout
from concurrent hostile modification; these are not secrets-vault or filesystem
sandbox boundaries.

```bash
export DEVSPECS_TELEMETRY=0
export DEVSPECS_HTTP_DISPATCH_CONFIG="$(cygpath -m /d/trusted/dispatch-profile.json)"
# Supply MY_DISPATCH_TOKEN through the caller's secret-management mechanism.
ds dispatch task:review --task-repo /d/planning --cwd /d/repo --agent-provider ExampleRuntime --read-only --detach
ds dispatch status 'dispatch_<id>'
ds dispatch resume 'dispatch_<id>'
```

`--read-only` is mandatory caller intent, not inferred from task text. Do not
use it for implementation tasks: a review is not a weaker successful substitute
for requested edits. The portable prompt explicitly requests inspection only,
no file changes/commits/lifecycle writes, and an unchanged result commit; it
asks the provider to fail an incompatible goal. This is a contractual request,
not enforcement of the remote worker's filesystem. Local admission refuses
edited-path claims and any result commit different from the source, and requires
a result commit for success in read-only mode. A dishonest provider's execution claims are not
authenticated by the request hash or by source=result alone.

For edited goals, the caller must explicitly use `--allow-edits`, the repository
must select `goal: edited`, and the trusted profile must select
`"read_only": false, "allow_edits": true`. Neither flag is inferred from task
text, and the flags are mutually exclusive. Existing read-only profiles and
pending records remain read-only. Changing a frozen dispatch's mode is refused.
Initial edited transfer supports SHA-1 source/result/tree OIDs only.

Provider-owned report/verification requirements may be added to that profile:

```json
{
  "read_only": false,
  "allow_edits": true,
  "report_required": true,
  "verification": {"profile": "node-unit", "strength": "suite"}
}
```

These are additional fields in the full profile above, not a standalone profile.
They are frozen into the existing portable request requirements. A successful
receipt must supply the requested report and passing verification at the exact
strength, with digest-checked inline report/test bytes. Those bytes are retained
as local artifacts. Passing verification remains a provider execution claim,
not a test independently rerun by the caller.

```bash
ds dispatch task:implementation --task-repo /d/planning --cwd /d/repo --agent-provider ExampleRuntime --allow-edits --detach
```

The CLI suppresses only its implicit native MCP default for an explicit
HTTP goal. Explicit MCP settings, operations, lane names, shell setup/verification,
verification labels/timeouts/strength flags, report paths, automatic/default-provider
fallbacks, and passthrough arguments are rejected rather than silently ignored.
In particular, native `--verify` is never reinterpreted as a provider profile.
Agent provider/model/effort remain portable wire options.
Native dispatch defaults are unchanged; both HTTP goal flags are refused for
non-HTTP providers. Both modes use the existing dispatch command group.

## Trust Before Authentication

Preflight performs local validation only. It checks actual Git origin and a
clean HEAD matching the trusted profile before reporting local adapter
capabilities. This does not assert live provider readiness. Dispatch checks both repositories before apply capture or state
publication. The exact origin, clean pinned HEAD, current profile, and frozen
binding are rechecked before **every** authenticated exchange, including the
remote capabilities request. TLS verification remains enabled; redirects and
transparent POST retries are disabled. Provider messages, bearer values, and
network error URLs are not included in transport errors.

All normal-HTTP local Git inspection, freezing, and admission use a sanitized
Git process environment, not the native driver's inherited `ExecRunner`.
Ambient `GIT_DIR`, `GIT_WORK_TREE`, alternate-object, and config injection
variables cannot redirect source inspection. The reported root must match the
physical repository root. Hooks, fsmonitor, external attributes files, credential
helpers, and automatic maintenance are disabled. Configured attribute filters,
textconv/external diff commands, and partial/promisor clones are unsupported and
refused before status/object inspection. Git receives no bearer environment.
The internal real `ds apply` capture also has a scrubbed environment, removes the
active token slot, bounds output and runtime, disables Git helpers, and forces
`DEVSPECS_TELEMETRY=0`. Native Waspflow subprocess behavior is unchanged.

Only the portable prompt and explicit supported inline context are sent, not
the full local apply payload, checkout paths, or v1 repository identity. A
`context_artifacts` reference must carry actual `content_base64` and
`media_type`, with matching artifact/revision IDs and SHA-256. The existing
freeze API verifies and embeds those bytes. Reference-only, external, unknown,
or mismatched inputs fail closed; the adapter does not fetch artifacts or
silently replace missing context with references. Ordinary apply output that
already embeds its task material in the prompt needs no artifact resolver.

## Recovery And Receipts

The existing v1 local request, handle, and normalized receipt formats remain
version 1. HTTP-private immutable records live separately under
`dispatches-http-driver-v2` in the caller-owned state home:

- `<id>.pending.json`: exact frozen POST bytes/hash, v1 request hash, canonical
  checkout/task roots, provider, endpoint/source/principal/CA/mode binding, and
  fixed recovery expiry.
- `<id>.accepted.json`: correlated accepted v2 handle, published separately.
- `<id>.evidence.json`: admitted, filtered receipt evidence, never provider prose.
- `<id>.receipt-binding.json`: immutable request/receipt hashes before bundle
  download, preventing changed receipt metadata from being substituted on recovery.
- `<id>.result.bundle`: transferred bundle, published only after independent
  Git verification. Requested inline report/test bytes are retained separately.

Publication stages and syncs a complete file before atomic non-replacing
publication. Pending state is published first, then its v1 recovery handle,
then POST. A crash before v1 handle publication cannot have submitted work.
A lost POST response leaves a recoverable handle; status reports `pending`
without resubmission. Explicit wait/resume/finalize may replay the **original
bytes and dispatch ID**, never regenerate a task or assume execution did not
happen. Provider idempotency retention must cover the one-hour recovery window.
Replays do not extend that window. Each execution requests `max_attempts=1`.

Credential rotation is permitted under the same principal: the current
profile's credential ID/token environment may change, while frozen
`caller.credential_id` remains the original value. Credential ID and token slot
are not hashed into semantic binding. Endpoint, exact source/commit, principal,
TLS trust digest, mode, and report/verification requirements must still match. Cached HTTP status/resume
also validates the current binding before returning local success.

The original execution checkout must intentionally remain clean at the original
source commit, including for cached status/resume. If the caller changes HEAD or
the working tree, observation is locally refused until the original binding is
restored. That refusal is not a provider execution failure, does not rewrite a
receipt into failure, and never automatically starts another execution.

Context cancellation and bounded local wait deadlines stop observation, not
the remote worker. There is no automatic cancel or new run after an ambiguous
outcome, expired recovery, or provider refusal. The transport's cancellation
contract remains available to the tagged pilots; the normal Driver interface
does not add a cancel command.

Receipts use existing Git/local-truth admission and protected lifecycle paths.
No checkout, worktree, index, user ref, or lifecycle promotion is performed.
The driver does not download external report/test artifacts or claim they have
been verified. TLS and independent Git verification supply different checks;
the request hash correlates frozen bytes, not a signature over result metadata.

## Edited Transfer

An edited success must include both `repository.result_tree` (raw 40-hex Git
OID) and `repository.result_bundle`, an HTTPS Artifact with ID `result-bundle`,
media type `application/x-git-bundle`, exact size, SHA-256, and URL
`<trusted-base>/dispatches/<id>/artifacts/result-bundle`. No-edit receipts must
omit both fields. Failed/cancelled executions cannot claim an edited result.
The normal consumer removes only these two agreed fields before applying the
unchanged strict draft-2 receipt schema; tagged pilots still reject them.

Downloads use the existing verified TLS session and current same-principal
credential, never an artifact-provided credential reference. Exact URL equality
is required before authentication: no alternate host, query, path, or redirects.
The transfer deadline is two minutes, separate from ten-second control calls,
and is also bounded by cancellation and recovery expiry. Size and SHA-256 are
stream-checked before any Git import; the result is capped at 32 MiB. HTTP 410
preserves existing state and fails the transfer, never resubmits accepted work.

The consumer accepts only canonical Git bundle v2 with one prerequisite equal
to source and one head `refs/dispatch/<id>` equal to result. The PACK header
must declare 1-10,000 objects. A separately bounded Git phase creates owned bare
repositories under the HTTP state namespace. It exports only objects reachable
from the pinned source using an owned ref/config and a scrubbed environment;
source config, hooks, filters, replacements, and promisor lazy fetching are not
used for that export. There are no source object hardlinks or source ref/index
writes. Object alternates, promisor pack files, and linked object entries are
refused; the source object-directory walk is capped at 200,000 entries.

The owned source bundle is verified and copied into quarantine. Result import
uses `bundle verify`, exact `list-heads`, `index-pack --strict --fix-thin`, and
full strict fsck. The result must have exactly one parent equal to source, and
its actual tree must equal `result_tree`. Only then is its ref fetched into the
owned quarantine. Exact complete `files_edited` must equal all changed paths
from a `--no-renames` Git diff, including both sides of a rename. Existing raw
diff, protected-history/root, and mode verification are retained; symlink and
gitlink changes are refused. The verified result bundle is retained in caller
state for separate caller-controlled use; no automatic checkout or promotion
is performed. Temporary quarantine repositories are removed after verification.

### Resource Gate

Compressed size and object count do **not** bound expanded object sizes,
delta-expansion memory, total disk consumption, or all Git subprocess resource
use. `index-pack --strict` is an integrity check, not a resource sandbox. The
Git phase has a two-minute deadline, one-thread settings and conservative pack
memory settings, but no OS-enforced memory/disk/process limits. This remains a
controlled, trusted-provider path, not hostile-bundle/customer-general readiness.
OS resource isolation or equivalent independently demonstrated bounds remain a
release gate; a compression-bomb safety claim would be incorrect.

The source export is additionally capped at 128 MiB of bundle output. Large
histories that exceed this limit fail the whole transfer. There is no shallow
fallback, alternate source, semantic downgrade, or new worker execution. All
these refusals preserve recoverable dispatch state. The checked-in provider
golden is wire-compatibility evidence only, not proof of a real edited run.

## Verification Scope

Automated tests cover local TLS transport, exact replay and token rotation,
strict provider-fixture compatibility, real Git bundle admission, protected
paths, and local environment isolation. They do not establish live provider
execution or OS-enforced resource isolation.

The optional external-apply integration test uses caller-selected
`DEVSPECS_HTTP_APPLY_BINARY` and `DEVSPECS_HTTP_APPLY_FIXTURE` against a synthetic
task repository. Without both test-only variables it skips. Record that skip
honestly; a fixture test is not evidence that a production deployment is ready.
