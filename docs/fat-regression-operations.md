# Fat Regression Operations

The fat-100 workflow is an opt-in maintainer regression gate for controlled
infrastructure. It is separate from the hosted smoke-5 pull-request gate because
the full corpus is large, includes local/private repositories, and needs stable
machine-level timing.

## Runner contract

Register a Linux x64 GitHub Actions runner with the custom label
`devspecs-fat-regression`. The runner must have Git, Go 1.25-compatible build
dependencies, enough disk for isolated indexes, and local full-history
checkouts for every pinned repository.

Configure these repository variables:

| Variable | Purpose |
| --- | --- |
| `DEVSPECS_FAT_REGRESSION_ENABLED` | Set to `true` only after the dedicated runner is online. |
| `DEVSPECS_FAT_ACTIVATION_MANIFEST` | Absolute runner-local YAML manifest for `recent` and `map`. |
| `DEVSPECS_FAT_SCAN_MANIFEST` | Absolute runner-local YAML manifest for `scan`. |
| `DEVSPECS_FAT_RESULTS_ROOT` | Absolute, dedicated runner-local directory for raw evidence. |
| `DEVSPECS_FAT_BASELINE_REF` | Accepted Git baseline, normally the latest promoted release tag. |
| `DEVSPECS_FAT_RETENTION_DAYS` | Optional raw-evidence retention; defaults to 14 days. |

Without the enable variable, scheduled runs skip. Manual dispatch remains
available for runner provisioning and accepts an explicit baseline ref.

## Corpus contract

Both manifests must describe the same 100 or more repositories. Every entry
must use a unique ID, an exact 40-character commit, `clone_mode: full`, and the
`fat` profile. The preflight verifies that each path is its Git root, `HEAD`
matches the pinned commit, history is not shallow, tracked files exist, and
there are no tracked edits, untracked files, or ignored build/generated files.
Ignored files can still affect repository configuration or tooling. Use dedicated clean
checkouts, not active development worktrees. Each activation entry has exactly
one `recent` and one `map` command; each scan entry has exactly one `scan`.

Run the same preflight directly on the runner before enabling the schedule:

```bash
go run ./scripts/ci/fat-regression preflight \
  --activation "$DEVSPECS_FAT_ACTIVATION_MANIFEST" \
  --scan "$DEVSPECS_FAT_SCAN_MANIFEST" \
  --minimum 100
```

Do not advance pins simply to make preflight pass. Refreshing a corpus is a
reviewed baseline operation and should preserve the old lock as historical
evidence.

## Evidence and privacy

Raw command output, per-repository cases, manifests, paths, and temporary index
homes stay under `DEVSPECS_FAT_RESULTS_ROOT`. The workflow deletes run folders
older than the configured retention period. Cleanup requires the marker created
by the workflow and only considers run-ID-shaped child directories; it will not
operate on `/`, the runner home, an unmarked directory, or unrelated children.

The public Actions artifact contains only aggregate repository/case counts,
timing statistics, shared failure counts, and the gate status. It contains no
repository identity, URL, path, pinned commit, stdout, stderr, or per-case
detail.

## Interpreting the gate

- `passed` means activation output matched the comparison baseline, the result
  schemas, execution modes, case identities and counts are internally consistent
  and match the successful preflight count, all three command exits were zero,
  and no scan case exceeded `floor(baseline_ms * 1.30) + 5000` milliseconds.
  This is ratio plus fixed slack, not separate ratio-AND-delta thresholds.
- `needs_review` means output changed, a material scan regression appeared, or
  one binary introduced a command failure.
- A command-level failure with the same classification in both scan roles is
  counted as shared debt. It does not make the candidate weaker by itself, but
  a nonzero command exit still requires operational review rather than a green
  run. Missing exit evidence also requires review.

Manual summary invocations must provide `--preflight-result` and the observed
`--activation-exit`, `--baseline-scan-exit`, and `--candidate-scan-exit` codes.
Do not substitute zeros for failed or unobserved commands. Empty, truncated,
duplicate or mismatched case sets are never accepted as passing evidence.
The workflow uses a run-specific public artifact path so a failed parse cannot
upload an older run's green summary.

The workflow never promotes changed output or advances the baseline. Review raw
local evidence, classify every quality delta, and update the accepted baseline
only through an explicit release decision. Automated reviewed-ground-truth
classification belongs to the follow-up regression-operations slice.
