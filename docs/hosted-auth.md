# Experimental Hosted Authentication

This bounded opt-in client adds remote metadata and explicit authentication, not
hosted sync or a deployed service. Local commands and help do not read the OS
vault, initiate device login or contact a configured hosted remote. No default
remote, automatic login, workspace propagation, push, pull, queue or remote task
lifecycle authority is added. Existing telemetry controls remain unchanged; these
commands do not add telemetry events or forward ambient headers/cookies.
Hosted remotes are hub-auth metadata, not Ghost HQ or dispatch-provider endpoints; dispatch configuration remains separate.

## Remote Metadata

```text
ds remote add hosted https://hub.example/base --tenant-id <approved-uuid> --repo-id <approved-uuid> --json
ds remote list --json
ds remote remove hosted --json
```

Run these commands inside a Git repository, and obtain the exact endpoint and
approved tenant/repo IDs from the operator. Config lives only in that Git root's
`.devspecs/config.yaml`. A `.devspecs` directory alone, including an ancestor or
global home directory, does not authorize reading or writing remote config;
non-Git directories fail with `repository_required` before config access. A fresh
Git repository needs no pre-existing `.devspecs` directory. For example:

```yaml
remotes:
  - version: 1
    name: hosted
    url: https://hub.example/base
    tenant_id: 33333333-3333-4333-8333-333333333333
    repo_id: 44444444-4444-4444-8444-444444444444
```

Each entry is versioned metadata, not authority. At most 16 names are allowed;
names contain lowercase letters, digits, underscores or hyphens and start with
a letter. IDs are lowercase UUIDs. URLs must be canonical HTTPS with lowercase
hosts, no trailing slash, userinfo, query, fragment, backslash, encoded path or
dot-segment ambiguity. A base path is supported and participates in **exact**
credential binding; aliases, alternate ports and other paths are not equivalent.

Add/list/remove perform no network or vault operation. Writes use the existing
atomic file replacement helper and preserve unrelated YAML nodes, including
unknown future keys/sections. Unsupported root/remote versions, unknown remote
fields, duplicate entries and oversized config fail before mutation. Concurrent
external config editing is not a compare-and-swap workflow. Remote removal does
not revoke/delete credentials; explicitly logout before removing metadata.
The remote-only path guard uses `Lstat` before reading and immediately before
writing. Existing `.devspecs` directories and `config.yaml` leaves must have the
correct directory/regular-file types and cannot be symlinks or Windows reparse
points. These checks reject static redirected paths; concurrent same-user path
replacement between validation and file operations remains outside this guarantee.
Real symlink regressions are non-Windows tests and remain a Linux CI execution
gate; native Windows junction behavior has not been exercised by those tests.

## Interactive Login And Logout

```text
ds login hosted --confirm-url https://hub.example/base --json
ds logout hosted --confirm-url https://hub.example/base --json
```

Verify the endpoint independently before confirming it. A repository edit cannot
silently select an origin for an ambient credential. Interactive vault access
requires a TTY and exact endpoint confirmation on every explicit login/logout.
Only explicit interactive `ds login` initiates the server-managed GitHub device
flow. It prints the exact `https://github.com/login/device` URL and user code to
stderr; opening a browser is optional and is not automated. The CLI never accepts
a GitHub access token as hub identity and never calls GitHub directly.

The client honors cancellation, returned polling intervals, slow-down and absolute
expiry (at most 15 minutes from receiving the start response). Each request has
a 10-second deadline, verified TLS, no redirects or environment proxy forwarding,
an 8 KiB response-header cap and 16 KiB JSON response cap. No insecure/CA override
flag exists. Server error bodies and transport/vault diagnostics are not echoed.

Only operator-pre-enrolled humans with active approved memberships can log in.
The server supplies stable principal/session IDs; the CLI verifies exact tenant,
repo, principal, session and expiry against the metadata read before storing the
new credential. Current roles authorize only `repo:metadata:read`, not future
`records:*` or mutation capabilities. An existing saved session must be explicitly
logged out before re-login. Cooperating interactive CLI processes serialize the
same binding through an existing file-lock implementation under DevSpecs home.

The lazy OS vault adapter is pinned to `github.com/zalando/go-keyring v0.2.8`.
It uses native Keychain, Secret Service or Credential Manager and fails closed
when unavailable, with no plaintext fallback. The private version-1 JSON record
is smaller than 2 KiB and contains the full exact base URL, tenant/repo binding,
opaque hub session token, principal/session IDs and expiry. Its namespace hashes
the complete endpoint; account selection also includes tenant/repo IDs. No record
or secret belongs in Git, repository config, logs, stdout or command arguments.
The pinned macOS Set implementation passes secret data on stdin, not argv.
Native OS vault calls may prompt/block; their backend has no context-cancellation
API. HTTP cancellation does not forcibly terminate an OS vault operation.

Metadata verification or vault-write failure triggers a bounded compensating
server logout. Unconfirmed compensation is an error requiring operator follow-up,
never a success claim. A normal logout deletes the saved credential only after
the exact version-1 server logout response is confirmed. Failed logout retains
the credential and names the retry; failed vault deletion after confirmed remote
logout also reports an error and can be retried. Expired saved sessions remain
available for explicit logout. A lost token response cannot be compensated by a
client that never received the token; operator revocation/bounded expiry remains
the recovery path.

## Explicit Headless Pair

Inject `DEVSPECS_REMOTE_TOKEN` through an approved operator/CI secret mechanism,
paired with `DEVSPECS_REMOTE_URL` equal to the **entire configured URL**, including
base path and port. Neither value comes from a token field in repo config.

```text
ds login hosted --json
```

This validates the explicit pair through the existing scoped metadata read. It
does not start device login, prompt, read/write a vault or persist the environment
credential. Missing, partial or mismatched pairs fail closed without fallback,
even when a vault contains another credential. A paired hub session can use
`ds logout`; scoped `sph_` service tokens instead require operator revocation,
which this client does not implement. The CLI cannot erase the parent's injected
environment variable. Protect environment injection under the operator's policy.

## Machine Output And Evidence

Every state command supports `--json`. Success is on stdout; errors are versioned
JSON on stderr with nonzero exit. Device instructions, when needed, remain on
stderr. Results use `schema: devspecs.remote.result`, `schema_version: 1`, `status`
and applicable metadata/identity fields; error objects contain fixed safe `code`
and `message`. No token, poll secret or private credential record is output.
Schemas are published in [remote result](../specs/remote-result.v1.schema.json)
and [remote config](../specs/remote-config.v1.schema.json). CLI URL validation is
stricter than the schema's basic URI shape check.

Focused evidence uses isolated config, minimal fake vaults and real local TLS
fixtures, including one complete command/device/poll/metadata/storage path.
No test accesses the native OS vault or performs live OAuth/provider/cloud calls.
Real platform-vault integration, hosted operation and broader release validation
remain owner-run gates; no production readiness or hosted synchronization is
claimed here.
