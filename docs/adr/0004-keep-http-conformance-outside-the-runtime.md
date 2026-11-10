# ADR-0004: Keep HTTP dispatch conformance outside the CLI runtime

## Status

Accepted for mock conformance. Live provider admission is pending.

## Context

Headless callers need a reproducible way to prove that task context can be
reviewed, dispatched, and checkpointed without an interactive terminal.
Transport conformance does not establish that a provider is authorized to
receive repository data or that a dispatcher can recover after a restart.

## Decision

Verify a checksum-pinned Linux archive with one `ds` binary. Use that binary
for apply and the explicit checkpoint, and use a test-only TLS loopback
provider for the HTTP leg. Send only reviewed synthetic input; do not send
the complete local apply output. Verify the remote receipt before storing
it locally. An accepted receipt is evidence, not authority to mutate the
task: the caller must explicitly checkpoint against expected task heads.

Publish the wire description and narrow JSON success contracts so consumers
can inspect the integration without a second executable or runtime provider
registration. No live endpoint, credentials, or provider option is enabled
by this proof.

## Consequences

- An offline Linux run can detect drift in the installed CLI and mock
  integration without giving the mock access to a real repository.
- Native HTTP dispatch, pending-run recovery, and broader machine error
  coverage still require proof. Passing loopback tests does not approve a
  live provider or complete the end-to-end delivery milestone.
