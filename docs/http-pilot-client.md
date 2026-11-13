# Experimental Read-Only HTTP Pilot Client

Available only in a candidate built with `-tags devspecs_http_loopback`.
Ordinary releases ignore these settings. This is not deployment approval,
a hosted-hub dependency, or a new orchestration provider default.

The caller explicitly selects an absolute local JSON config path with
`DEVSPECS_HTTP_PILOT_CONFIG`. Nothing is read from repository configuration.
Use a separate existing absolute `DEVSPECS_HOME` and keep the trusted
config outside the repository. Do not set the loopback endpoint variable
at the same time: conflicting modes fail closed.

Example template, deliberately disabled:

```json
{
  "schema": "devspecs.dispatch.pilot_config",
  "schema_version": 1,
  "enabled": false,
  "endpoint": "https://your-approved-app.fly.dev/devspecs/v2",
  "source_url": "https://github.com/your-org/pilot-fixture.git",
  "principal_id": "your_stable_principal",
  "credential_id": "your_current_credential",
  "token_env": "DEVSPECS_PILOT_TOKEN"
}
```

Do not enable it or provision the referenced token until pilot access is
approved. The file contains non-secret metadata only; the caller supplies
only the named environment token through the agreed secret mechanism.
The token never enters pending state, receipt evidence or errors.

Use the existing `dispatch`, `dispatch status`, `dispatch resume`, and
`dispatch receipt` commands. The initial command reads the exact, explicitly
reviewed draft-2 wire request from stdin, not the entire local apply output.
The request must name the configured source URL and caller, and match the
clean local checkout's source commit. Use only inline context for this
pilot. Keep the dispatch ID and frozen bytes for recovery.

TLS uses system trust by default. A separately selected absolute `ca_file`
is available for offline fixtures. DNS hostname validation stays on, HTTPS
is required, redirects are refused, and no ambient HTTP proxy is used.
Explicit TLS loopback with a CA is supported for offline conformance; the
old synthetic-credential loopback mode stays loopback-only.

On rotation, update the credential metadata and caller-owned token source,
not the frozen request. Pending runs remain bound to their original
endpoint, trust source, principal and source URL. The existing bounded
recovery rules are unchanged. Status never resubmits.

This first client is read-only: a successful result must name the source
commit as its result. Changed commits are not fetched or accepted, even
if otherwise available locally. No result-bundle field or remote Git
credential is introduced. A verified receipt never checkpoints on its own.
