# Waspflow And Shell-First Repository Support

## Goal
Make DevSpecs useful on the first cold interaction in shell-first repositories, then determine whether DevSpecs tasks and Waspflow lanes need a direct integration or already compose cleanly through their existing structured surfaces.

## Product Invariants
- First-run output must be at least as useful as the indexed steady-state output. A fast docs-only fallback is not acceptable.
- Solve the general shell-repository gap before adding Waspflow-specific ranking or commands.
- Preserve useful Git-derived `recent` behavior while adding implementation and behavior-test evidence.
- Do not add CLI surface until an end-to-end composition experiment proves an existing JSON/file contract is insufficient.
- Waspflow verification receipts are evidence, not authority to auto-promote a DevSpecs checkpoint.

## Investigation Baseline
- Waspflow repository: `https://github.com/tnunamak/waspflow`
- Pinned commit: `3d9634af32a5d8791671e0a320eedc2047141a2c`
- DevSpecs probe binary: `ddc911cf96ab71367f0e370ec7dbf048a5eab660`
- Inventory: 81 files; `.git` and `lib/generated` skipped.
- Cold `scan`: 1,562 ms wall time; 20 Markdown artifacts; 0 source-context artifacts; 0 test-case artifacts.
- Cold `recent`: 1,054 ms and five coherent Git-derived topics. Preserve or improve this output.
- Cold `map`: 1,538 ms, but boundaries are mostly generic directory/doc clusters and miss the runtime architecture.
- Five `find` probes returned only background/design documents with role diversity 1 and warned that no primary implementation surface was visible.

## Ground-Truth Waspflow Surfaces
- CLI and lifecycle dispatch: `bin/waspflow`
- Lane state, events, execution, fan-in, worktrees, escalation, selection, billing, and receipts: `lib/*.sh`
- Provider contract and adapters: `lib/providers/*.sh`
- Behavioral verification: `scripts/verify.sh`, `scripts/live-smoke.sh`, and `scripts/live-soak.sh`
- Product intent: `README.md`, `skill/SKILL.md`, and `docs/design/**`
- Planned, not implemented: `docs/mcp.md`

## Track Order
1. W01 pins baseline artifacts and a human-reviewed quality oracle.
2. W02 makes shell implementation discoverable during the default cold scan.
3. W03 extracts useful shell semantics and behavioral tests.
4. W04 proves `map`, `find`, `task`, and `recent` quality across Waspflow and other shell-first repositories.
5. W05 defines the DevSpecs-to-Waspflow contract and makes the product/no-product decision.
6. W06 prototypes the smallest integration only when W05 promotes it.
7. W07 closes worktree, concurrency, quality, and performance gates.

## Shared Release Gates
- Compare released `v1.3.0`, the current v1.4.0 release-candidate `main`, and the exact candidate commit on independent cold `DEVSPECS_HOME` directories.
- Pin full Git history and exact repository commits for every canonical fixture.
- Run self-vs-self determinism before interpreting candidate deltas.
- Require no weaker first result for `recent`, `map`, `find`, or `task`.
- Add Waspflow to the canonical regression manifests only after the oracle is reviewed; include at least two unrelated shell-first repositories to prevent overfitting.
- Run focused Go tests, `go test ./...`, smoke-5 plus shell fixtures on pull requests, and fat-25/fat-100 manually before promotion.

## Current Decision
Proceed with W01. Do not implement a Waspflow-specific command or MCP adapter yet.
