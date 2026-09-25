# Development

```bash
git clone https://github.com/devspecs-com/devspecs-cli.git
cd devspecs-cli
bash scripts/dev.sh --worktree -- go test -p 1 -count=1 -timeout 30m ./...
bash scripts/dev.sh --worktree -- go run ./cmd/ds --help
```

The development launcher keeps local builds away from the stable
`~/.devspecs` index. Worktree scope is the default and gives each DevSpecs CLI
checkout a deterministic home under `~/.devspecs-dev/worktrees/`. Set
`DEVSPECS_DEV_HOME_ROOT` only when the managed development parent must live
elsewhere:

```bash
bash scripts/dev.sh --worktree -- go run ./cmd/ds doctor
```

Use a named channel when one preview binary should retain an index across
builds or launch a long-lived agent that will work in several target repos:

```bash
bash scripts/dev.sh --channel preview -- /path/to/preview/ds doctor
bash scripts/dev.sh --channel preview -- codex
```

From another repository, invoke the launcher by its absolute path; the child
keeps that repository as its working directory while the home remains tied to
the selected DevSpecs development lane.

The launcher sets `DEVSPECS_HOME` only for its child process and defaults
development telemetry off. An explicit telemetry mode such as
`DEVSPECS_TELEMETRY=debug` is preserved for endpoint testing. PowerShell uses
the same resolver and path contract:

```powershell
.\scripts\dev.ps1 --worktree -- go run ./cmd/ds doctor
```

The first isolated run starts with an empty index and rebuilds from repository
sources. Do not copy `~/.devspecs/devspecs.db` into the development home: that
can carry preview schema migrations and benchmark state into the new lane.
Existing custom homes remain valid when selected explicitly with
`DEVSPECS_HOME`; setup does not move, adopt, delete, or downgrade them. Run
`ds doctor` before deciding whether an older custom home is compatible.

Useful checks:

```bash
gofmt -l .
go vet ./...
staticcheck ./...
```

To enable the repo pre-commit hook:

```bash
make hooks
```

The hook runs `go vet`, `staticcheck`, `gofmt -l`, and by default
`go test -p 1 -count=1 -timeout 30m ./...` in a disposable DevSpecs home.

## Releasing

Releases use GoReleaser via GitHub Actions.

Before tagging a release candidate, run the local gate:

```bash
gofmt -l .
go vet ./...
staticcheck ./...
bash scripts/dev.sh --worktree -- go test -p 1 -count=1 -timeout 30m ./...
```

Merge the reviewed release PR and wait for the required main-branch checks
before tagging the intended version. Check companion documentation and release
notes first; do not tag a feature branch to bypass validation.
