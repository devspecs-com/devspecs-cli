# Task waspflow-shell-support W02 Plan

## Goal
Admit shell implementation on the first cold scan.

## Starting Evidence
- `sourceLanguage` already labels `.sh`, `.bash`, and `.zsh` as shell.
- Default source-context admission does not accept those extensions.
- First-party source discovery recognizes shell extensions, but only runs behind an experimental option.
- Extensionless executable scripts such as `bin/waspflow` are not recognized by path extension.

## Expected Change Surface
- `internal/adapters/sourcecontext/sourcecontext.go`
- `internal/scan/first_party_source_context.go`
- default scan/auto-index option wiring in `internal/commands`
- focused adapter, scan, and command tests

## Work
- Admit `.sh`, `.bash`, and `.zsh` in first-party implementation and test roots during the default cold scan.
- Detect extensionless shell entrypoints conservatively from an executable file plus a supported shell shebang.
- Include implementation roots such as `bin`, `lib`, and provider subdirectories without admitting generated/vendor output.
- Make the first-party source path part of the product default where required by `recent`, `map`, `find`, and `task`, not a warm-only enhancement.
- Preserve candidate caps, ignore behavior, cancellation, and deterministic ordering.

## Acceptance
- A cold Waspflow scan includes `bin/waspflow`, representative `lib/*.sh`, `lib/providers/*.sh`, and `scripts/verify.sh` as typed source evidence.
- `lib/generated/**`, fixtures, vendored code, and arbitrary executable data files remain excluded.
- Extensionless files without a supported shebang remain excluded.
- Repeated cold scans produce identical normalized output.
- Waspflow cold wall time does not regress by more than 20% or 500 ms, whichever allowance is larger, before W04 quality work.
- Existing canonical repositories show no artifact-kind or first-result quality regression.

## Test Standards
- Use testify assertions.
- Keep one behavioral case per test with Prepare, Act, Assert structure.
- Assert collection length before index-by-index values; do not assert whole result slices.
- Use real temp repositories and files where practical; keep mocks minimal.

## Decision Gates
- Promote: shell implementation is present on the first cold result with bounded cost and no noise expansion.
- Improve: core files are admitted but shebang, roots, or ignore behavior needs another bounded slice.
- Rework: admission depends on warm state or broadly indexes shell noise.
- Rollback: non-shell repositories become weaker or materially slower without compensating quality.
