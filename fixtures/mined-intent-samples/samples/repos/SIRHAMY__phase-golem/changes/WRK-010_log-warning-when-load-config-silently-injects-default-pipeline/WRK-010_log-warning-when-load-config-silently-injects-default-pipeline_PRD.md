# Change: Log warning when load_config silently injects default pipeline

**Status:** Proposed
**Created:** 2026-02-13
**Author:** Orchestrator (autonomous)

## Problem Statement

When `load_config()` in `config.rs` finds no pipelines defined — either because no `orchestrate.toml` exists or because the file omits the `[pipelines]` section — it silently injects a default "feature" pipeline (a built-in pipeline configuration used when no explicit pipelines are configured) via `populate_default_pipelines()`. This happens without any log output, making it invisible to the user that they're running with automatically populated defaults rather than explicitly defined configuration.

This is a debuggability problem. When a user wonders "why is my item going through these specific phases?" or "where is this pipeline defined?", there's no indication in the logs that the pipeline was auto-generated. The silent injection makes it harder to diagnose configuration issues and understand orchestrator behavior.

## User Stories / Personas

- **Orchestrator operator** - A developer running `orchestrate run` who hasn't defined explicit pipelines. They should see a warning in the log output indicating that the default pipeline is being used, so they know their config is incomplete and can choose to define explicit pipelines if needed.

## Desired Outcome

When `load_config()` injects the default "feature" pipeline because no pipelines are configured, the `load_config()` function should emit a `log_warn!()` message indicating that no pipelines were defined and the default "feature" pipeline is being used. The warning should appear once per `load_config()` call and be visible at the default log level (the default is `Info`, which includes `Warn`-level output).

### Example Messages

**Scenario 1 — No config file exists:**
```
[config] No orchestrate.toml found; using default 'feature' pipeline
```

**Scenario 2 — Config file exists but no pipelines defined:**
```
[config] No pipelines defined in orchestrate.toml; using default 'feature' pipeline
```

## Success Criteria

### Must Have

- [ ] A `log_warn!()` message is emitted in `load_config()` when `config.pipelines.is_empty()` is true before `populate_default_pipelines()` inserts the default pipeline
- [ ] The warning message includes the pipeline name ("feature") so the user knows what was injected
- [ ] The warning distinguishes between the two scenarios: no config file found vs. config file found but no pipelines defined. The distinction is implemented in `load_config()` (not `populate_default_pipelines()`) since `load_config()` has context about which code path was taken.
- [ ] Existing tests continue to pass (log output to stderr from `log_warn!` does not affect test assertions)

### Should Have

- [ ] The warning message references `orchestrate.toml` so the user knows where to define explicit pipelines (e.g., "No pipelines defined in orchestrate.toml")

### Nice to Have

- [ ] None identified

## Scope

### In Scope

- Adding `log_warn!()` calls in `load_config()` in `config.rs` to warn when the default pipeline is injected
- Differentiating the message between "no config file" and "config file exists but no pipelines"

### Out of Scope

- Changing the default pipeline behavior itself
- Adding new config validation rules
- Logging pipeline phase details at info/debug level (could be a follow-up)
- Changes to the `validate()` function
- New tests for log output verification (existing tests validate behavior; log output is a side effect)

## Non-Functional Requirements

- **Observability:** The warning should use the existing `log_warn!()` macro from `log.rs`, consistent with the rest of the codebase. At log levels below `Warn` (i.e., `Error` only), the warning will be suppressed; this is expected behavior.

## Constraints

- Must use the existing logging infrastructure (`log_warn!` macro from `log.rs`)
- The `log_warn!` macro is exported via `#[macro_export]` in `log.rs`, making it available at the crate root. It can be invoked as `log_warn!()` directly from `config.rs` without additional imports.
- Each command handler in `main.rs` calls `load_config()` once independently (lines 258, 563, 667, 752). Users will see the warning once per command invocation, which is acceptable.

## Dependencies

- **Depends On:** None
- **Blocks:** None

## Risks

- [ ] Minimal risk — this is a pure addition of log output with no behavioral changes

## Open Questions

- None — the scope is clear and well-bounded

## Assumptions

- The `log_warn!` macro is available at the crate root via `#[macro_export]` and can be called directly from `config.rs` without additional `use` statements (confirmed: this is standard Rust `#[macro_export]` behavior, and other modules like `main.rs` already use these macros)
- Two distinct messages for the two scenarios is preferred over a single generic message, for better debuggability
- The logging calls belong in `load_config()` rather than `populate_default_pipelines()`, since only `load_config()` knows whether a config file was present

## References

- `config.rs:207-244` — `load_config()` and `populate_default_pipelines()`
- `log.rs` — logging macros (`log_warn!`, `log_info!`, etc.)
- WRK-009 — Related item: "Call validate() on the no-config-file default path for defense-in-depth" (touches the same code path)
- WRK-008 — Related item: "Add helper constructor for default_feature_pipeline to reduce verbosity"
