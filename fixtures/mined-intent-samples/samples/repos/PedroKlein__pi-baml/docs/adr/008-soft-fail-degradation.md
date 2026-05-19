# ADR-008: Soft-Fail When BAML Runtime Unavailable

## Status
Accepted

## Context

`@boundaryml/baml` includes native Rust NAPI binaries. On unsupported platforms or with corrupted installations, the native module fails to load. If pi-baml hard-fails, the entire extension doesn't load — tools aren't registered, EventBus event never fires, and dependent extensions may break silently.

## Decision

Soft-fail: pi-baml always loads successfully. If the BAML runtime can't be imported, it emits `{ available: false }` on the EventBus and registers tools that return helpful error messages.

<!-- stripped fenced code block: typescript -->

## Alternatives Considered

1. **Hard-fail** — let the extension crash during loading. Pi shows it in the extension errors list. Problem: dependent extensions don't know pi-baml failed — they just never receive the EventBus event, leading to silent null references.

## Consequences

- Extensions that depend on pi-baml can check `baml.available` and fall back gracefully
- Tools are still registered (visible in tool list) but return clear error text
- The agent sees a meaningful error if it tries to use BAML on an unsupported platform
- No silent failures — the problem is surfaced at every interaction point
