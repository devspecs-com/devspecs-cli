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
for the tested boundaries. The success schemas are additive and do not
describe checkpoint drafts or error output. Do not parse error prose as a
stable protocol. Native HTTP dispatch, cross-process resume, full machine
error coverage, and live-provider acceptance are separate gates.
