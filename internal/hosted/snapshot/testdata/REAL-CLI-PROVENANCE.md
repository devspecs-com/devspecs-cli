# Synthetic Real CLI Fixture

The `real-cli` directory is an unchanged Git-object export of the main-owned,
synthetic-only corpus at commit `153f009c654163fb4861d3c13b5b0ce8b6057118`, parent
`2e8e5563036e4fd3bf72d27343c40abdbb940eb5`. Main generated it using the existing
CLI at `15b4465`, including custom task paths, two slices, a schema-3 checkpoint
JSON/Markdown pair, a schema-1 named-thread definition, a custom composed ADR,
and a second task under the default task path. Fourteen committed files were
exported with `git -c core.autocrlf=false -c core.eol=lf -c core.attributesFile=NUL
archive`. The first ordinary archive attempt did apply Windows newline conversion;
it was replaced before acceptance. The imported task manifest's raw Git blob ID
is checked against original `1844a167476939bb52b103499893a2bffd45816a` in tests.
Scoped `-text` attributes preserve the fixture's original bytes on checkout.

There is no committed `.devspecs/config.yaml`. Absolute machine paths inside
record content are historical metadata, never source instructions or durable
identity. The ADR's local index artifact ID was not persisted into its Markdown.

Tests import these bytes into isolated temporary Git repositories and configure
their policy scope explicitly. They do not operate on the original fixture,
customer repositories, a live service or the implementation worktree's index.

## Started / Continue

`real-continue` imports only `devspecs/tasks/normal-edited-proof` from main's
synthetic commit `b729c7933f08cadf9b2b3ffae34254f93f18a810`, created with CLI
`15b4465`. It contains the actual schema-3 started/continue checkpoint
`cp_20261113T123704Z_e01_started` and the generated task latest-reference triple.
No other tasks from that later corpus revision were imported.

## Workspace / Change

`real-workspace` imports three files from synthetic commit
`0ffda93a048ff9195f4c0f369490aeb0d8aa3baf`, generated using main's existing
`ds-hosted-auth-gitbash.exe` (`15b4465`) in a new isolated D: home/repository with
both telemetry environment settings `0`. Commands were `workspace init .` and
`workspace change create ... --workspace . --repos app`. Only the generated
`devspecs` files were committed/exported; the nested synthetic `app` Git marker
was used for CLI workspace enrollment and is not part of the imported tree.
All imports use the conversion-disabled archive flags above.

## Explicit Custom Workspace Artifacts

`real-workspace-custom` imports three real CLI-generated files from the isolated
synthetic root commit `b28e8aa81473095af5d793d7db38896ec01dc0bf`. Its committed
manifest declares `artifact_dir: planning`. Tests require the separately approved
policy `artifact_root: planning`; a mismatched or omitted approval fails whole.

## Independent Semantic Goldens

`semantic-goldens` contains independently generated private service outputs for
all four fixture directories, using repo ID `repo-one`, the policy ID in
`policy-v1.json`, and `refs/heads/main`. Only the envelope commit is normalized to
forty `1` characters for comparison. These are not reachable source claims.
Tests compare exact canonical bytes and pin each SHA-256. Service readers were
mirrored locally in `contract.go`, `records.go`, `workspace.go` and the committed
Git collector, retaining canonical field order and raw-byte semantics rather
than introducing a second corpus registry or public SDK contract.
