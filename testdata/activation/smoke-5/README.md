# PR smoke-5 recent quality gate

This tracked public corpus is the reproducible PR form of the local smoke-5 gate.
It pins full-history commits across Python, Go, Rust, and web repositories and
compares cold `ds recent --max-areas 5` JSON with reviewed normalized goldens.

The `Activation Smoke` workflow runs it for every pull request to `main`.
An intentional output change must be reviewed before updating the goldens; do
not use `--activation-update` merely to make the workflow pass.

Local comparison:

```powershell
$env:DEVSPECS_SMOKE_ROOT = 'C:\tmp\devspecs-smoke-5'
go run .\scripts\ci\prepare_activation_smoke.go .\testdata\activation\smoke-5\manifest.json
go build -o C:\tmp\ds-smoke.exe .\cmd\ds
C:\tmp\ds-smoke.exe eval .\testdata\activation\smoke-5\manifest.json `
  --activation-matrix `
  --activation-profile skinny `
  --activation-clone-mode full `
  --activation-index-state cold `
  --activation-golden-dir .\testdata\activation\smoke-5\goldens `
  --json
```
