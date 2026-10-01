$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$childDir = (Get-Location).Path
& go -C $repoRoot run ./scripts/dev-env --source-root $repoRoot --child-dir $childDir @args
exit $LASTEXITCODE
