# Task install-self-update-utilities G03 Result

## Summary
- Target: `G03` - Document restart shell or IDE after install and upgrade
- Outcome: documented install/upgrade verification, `ds update`, and shell/IDE terminal restart guidance in the README and installer success messages.

## Changed Files
- `README.md`
- `install.sh`
- `install.ps1`

## Tests
- `C:\Program Files\Git\usr\bin\bash.exe -n install.sh`
- PowerShell parser tokenization for `install.ps1`
- `rg "ds update|ds version|restart your shell|IDE terminal|Install Troubleshooting" README.md install.sh install.ps1 -n`
- `git diff --check`

## Decision
- Promote. G track is complete unless later launch smoke finds install/update copy problems.

## Follow-up
- Move to `J01` after release-priority confirmation: interactive tooling selection and background indexing through `ds init`.

## References
- `G00-index.md`
- `G03-document-restart-shell-or-ide-after-install-and-plan.md`

## Checkpoints
- Use `ds task checkpoint install-self-update-utilities --target G03` to append structured evidence.

### Checkpoint
- Created At: 2026-06-16T14:01:50Z
- Stage: validated
- Decision: promote
- Source: `checkpoints/20260616-140150-validated.md`
- Structured Evidence: `checkpoints/20260616-140150-validated.json`
- What changed: Added README update/troubleshooting guidance for ds update, ds version verification, and restarting shells or IDE terminals after install/upgrade. Updated install.sh and install.ps1 success messages to point to ds version and restart guidance.
- Evidence for decision: 4 file(s) read; 4 file(s) edited; 4 test command(s)
- What remains: -
- Next iteration: promote to the next slice
- Files read:
  - `devspecs/tasks/install-self-update-utilities/G03-document-restart-shell-or-ide-after-install-and-plan.md`
  - `README.md`
  - `install.sh`
  - `install.ps1`
- Files edited:
  - `README.md`
  - `install.sh`
  - `install.ps1`
  - `devspecs/tasks/install-self-update-utilities/G03-document-restart-shell-or-ide-after-install-and-result.md`
- Tests run:
  - `C:\Program Files\Git\usr\bin\bash.exe -n install.sh`
  - `PowerShell parser tokenization for install.ps1`
  - `rg ds update/ds version/restart guidance in README.md install.sh install.ps1`
  - `git diff --check`
