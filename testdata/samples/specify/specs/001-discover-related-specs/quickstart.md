# Quickstart (implementer): Probabilistic related specs

**Plan**: [plan.md](./plan.md) · **Contracts**: [contracts/cli-ds-related.md](./contracts/cli-ds-related.md)

## Prerequisites

- Go **1.25+** toolchain
- Git (for fixtures and HEAD/commit integration)
- Working copy of **`devspecs-cli`** repository

## Orientation

1. Read [spec.md](./spec.md) (behavior) and [`testdata/samples/cursor/probabilistic_related_specs_481c4b3f.plan.md`](../../../cursor/probabilistic_related_specs_481c4b3f.plan.md) (technical checklist).
2. Read codex authoritative mining constants: [`testdata/samples/codex/PLAN.md`](../../../codex/PLAN.md) §Mining Behavior.
3. Skim existing patterns:
   - `internal/commands/resume.go` — Cobra layout
   - `internal/commands/scan.go` — `--quiet`
   - `internal/commands/init.go` — hooks / markers
   - `internal/store/` — schema + queries
   - `internal/scan/scan.go` — revision insert
   - `internal/repo/repo.go` — git operations

## Implementation order (suggested)

1. **Schema + store**: Version 4, new tables, `store_test` expectations, `UpsertFileLink`, `RelatedArtifactsForFile`, workon session helpers.
2. **Pure merge**: `internal/mining` unit tests for additive + cap + bucket thresholds.
3. **Scan**: pass `HeadCommit` into revision insert (+ test).
4. **Mining collectors**: git + text + workon-branch emission; conservative caps for `--all`.
5. **Commands**: `workon`, `mine`, `related`; register in `cmd/ds/main.go`.
6. **Hooks**: refactor `init` installers; extend idempotency tests.
7. **Fixtures**: git scenarios from PLAN §Tests + `go test ./...`.

## Local verification

```bash
cd /path/to/devspecs-cli
go test ./...
go run ./cmd/ds/ -- help
```

Run `ds mine --recent --json` and `ds related <file> --json` in a small temp repo; compare JSON keys to [contracts/cli-ds-related.md](./contracts/cli-ds-related.md).

## Definition of done (planning slice)

- All checklist bullets in [`probabilistic_related_specs_481c4b3f.plan.md`](../../../cursor/probabilistic_related_specs_481c4b3f.plan.md) have corresponding tests or traced code paths.
- No duplicate hook blocks after double `ds init --hooks`.
- Existing golden JSON tests unchanged except intentional additive fields with review.
