# Headless HTTP dispatch conformance

This developer check uses an installed `ds` for `apply --json` and
`task checkpoint --input-json --json`. The intervening HTTP exchange is
test-only and runs on TLS loopback. There is no supported HTTP provider
configuration or live partner access implied by the check.

## Run on Linux

Use an archive and checksum list from the same trusted build or release.
The candidate must include guarded checkpoint input and apply budget output;
an older released binary is not a substitute. A snapshot can use the same
archive layout with a top-level `ds` and a standard SHA-256 checksum line.
Run from a checkout matching the candidate. Git is required for the fixtures.

```sh
sh scripts/ci/verify-headless-dispatch.sh "$archive" "$checksums"
```

For an offline container, compile the test runner before entering it
(`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -p 1 -c -o orchestration.test ./internal/orchestration`
for an x86-64 image). Pass it as the third argument. Mount the inputs,
including the script at its checkout-relative path and the sibling
`specs/` directory, read-only. Provide an executable temporary directory; no outbound
network, TTY, credentials, or existing DevSpecs home are required.

## What it checks

- The pinned archive supplies one installed `ds` and no companion dispatcher.
- Apply and checkpoint stdout validate against their JSON success schemas.
  The caller checks the plan bytes against the reported
  digest, and sends only reviewed synthetic input.
- The loopback transport verifies the terminal receipt before storage. The
  task stays unchanged until an explicit caller checkpoint. Stale checkpoint
  replay exits nonzero and leaves the existing evidence untouched.
- Focused transport cases cover cancellation, retry with the same
  idempotency key, incompatible schemas, and invalid receipt rejection.

Use the [HTTP wire description](../specs/dispatch-http-v2.openapi.json),
[apply success schema](../specs/apply-output.v1.schema.json),
[checkpoint input schema](../specs/task-checkpoint-input.v1.schema.json),
and [checkpoint write success schema](../specs/task-checkpoint-output.v1.schema.json)
for the tested boundaries. Do not parse error prose as a stable protocol.

## Native loopback experiment

The same experimental build also contains the explicitly configured
[read-only pilot client](http-pilot-client.md). Offline pilot tests select
their own local TLS endpoint and synthetic credentials; they do not call
Fly or establish live provider readiness.

The production build still has no HTTP provider. For a developer-only
offline check, build both the Linux `ds` and the precompiled test runner
with `-tags devspecs_http_loopback`, then use the same archive verifier.
The caller-restart test starts a fixture provider, kills the first CLI
process after admission, and exercises recovery with rotated synthetic
credentials through another installed CLI process.

In the tagged build only, select the experiment with
`DEVSPECS_HTTP_LOOPBACK_ENDPOINT` (HTTPS literal loopback IP) and
`DEVSPECS_HTTP_LOOPBACK_CA` (absolute path to the fixture's PEM trust anchor).
Use an existing, disposable, absolute `DEVSPECS_HOME` and trusted fixture
repositories in a network-disabled environment. Repo config cannot opt in.
In this tagged build only, `DEVSPECS_HTTP_LOOPBACK_CREDENTIAL` selects a
built-in fixture slot: `initial` (default), `rotated` (same principal, new
credential), or `other` (unrelated principal for denial tests). This is not
a bearer-token input. Never use real credentials or customer data.

`ds dispatch task:<id> --cwd <fixture-checkout> --detach` takes the
exact reviewed draft-2 v2 request on stdin, including the original
`caller` binding. It does not send the local apply output,
read the task repository, or use a Git remote for source approval. The
request must use a synthetic `.invalid` source URL, the clean checkout's
pinned commit, and only inline artifacts. Keep the dispatch ID before
submitting. The experiment reuses `dispatch status`, `dispatch resume`,
and `dispatch receipt` with that ID; it adds no command or flag.

Pending state is bounded, immutable, and written before POST. It binds the
frozen bytes, digest, checkout, endpoint, trust anchor and provider name.
The frozen principal must match the current fixture slot. Rotation changes
only the Authorization header; the original credential ID stays in the
request and the verified receipt. No bearer is stored. Missing or different
capability revisions fail before POST. Use a fresh disposable home:
draft-1 records do not carry the required draft-2 binding.
`status` never submits. If no handle was stored, `resume` sends the exact
original bytes and key, subject to a one-hour recovery deadline and a
provider-advertised idempotency retention of at least one hour. If a handle
was stored, `resume` only monitors. This proves caller process recovery,
not provider crash recovery or power-loss durability. Expiration blocks
replay; it does not delete state. Retain the archive and discard the
disposable home when the proof is finished. Never recycle an expired ID.

The tagged workflow's [dispatch result](../specs/dispatch-loopback-result.v1.schema.json)
and guarded [checkpoint errors](../specs/task-checkpoint-error.v1.schema.json)
use versioned JSON on stdout after CLI argument parsing, with exit 1 for
handled errors. A valid terminal provider outcome (even failed or cancelled)
is evidence and uses exit 0; check the state rather than treating it as a
promotion. The [checkpoint draft](../specs/task-checkpoint-draft.v1.schema.json)
is read-only and keeps its existing JSON shape. No provider can checkpoint.

Do not ship the experimental tag or use this with a live provider. The
ordinary Waspflow path and human output are unchanged.
