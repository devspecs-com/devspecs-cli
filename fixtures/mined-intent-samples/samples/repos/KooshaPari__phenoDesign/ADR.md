# Architecture Decision Records -- @kooshapari/design

**Last Updated:** 2026-04-04  
**Status:** Active Document

---

## ADR-001: CSS-First, Framework-Agnostic Token Delivery

**Status:** Accepted  
**Date:** 2026-03-26

### Context

Phenotype projects span multiple UI frameworks and toolchains (VitePress, React, plain HTML, charting libraries). Design tokens must be consumable in all of them. The lowest-common-denominator interop layer for styles is CSS custom properties, not a specific JS framework's theming API.

### Decision

Ship all palette tokens as CSS custom properties in `css/keycap-palette.css` (the primary delivery vehicle). Provide a TypeScript/ES module mirror (`src/tokens.ts`) as a secondary convenience layer for programmatic access. No JavaScript is required to apply the palette.

### Alternatives Considered

- **Tailwind CSS config:** Ties consumers to Tailwind; breaks for non-Tailwind projects.
- **JS-only tokens (no CSS):** Requires a runtime and CSS-in-JS or manual var injection.
- **CSS Modules:** Scopes would conflict with third-party theming (VitePress, etc.).

### Consequences

Any project can apply the Keycap palette with a single `@import` or `<link>` tag. No build-tool pipeline required for basic usage. TypeScript exports remain available for advanced use cases (charts, canvas, dynamic theming).

---

## ADR-002: W3C DTCG JSON as Source of Truth

**Status:** Accepted  
**Date:** 2026-03-26

### Context

Design tokens need a standardized, tool-interoperable format so that Figma plugins, Style Dictionary pipelines, automated contrast checkers, and future tooling can all parse the same source file without bespoke adapters.

### Decision

`tokens/keycap.json` uses the W3C Design Token Community Group (DTCG) specification as the single authoritative token source. Every token entry uses `$value`, `$type`, and `$description` fields. The CSS and TypeScript files are derived representations.

### Alternatives Considered

- **Style Dictionary native format:** Proprietary; not recognized by Figma natively.
- **Figma tokens JSON (plugin format):** Figma vendor lock-in; less interoperable.
- **Hand-maintained CSS without a source JSON:** No single source of truth; drift risk.

### Consequences

Tokens are parseable by Style Dictionary, Figma Token Studio plugin, and any W3C DTCG-compliant tool. Adding new downstream output formats (Sass vars, Android XML, iOS Swift) requires only a new Style Dictionary transform, not re-authoring tokens.

---

## ADR-003: WCAG AA as Minimum Contrast Standard

**Status:** Accepted  
**Date:** 2026-03-26

### Context

The Phenotype ecosystem serves developer tooling and documentation. Color choices must be accessible to users with low-contrast sensitivity. Two standards exist: WCAG AA (4.5:1 for normal text) and WCAG AAA (7:1 for normal text).

### Decision

Every foreground/background combination used for readable text in the Keycap palette SHALL meet WCAG AA (4.5:1 minimum). WCAG AAA is not mandated because it would over-constrain brand color choices (e.g., the teal accent `#7ebab5` cannot meet AAA against white while remaining recognizable as teal).

Key verified pairs:
- Dark mode: `--kc-text-1` (#f6f5f5) on `--kc-bg` (#090a0c) -- ratio ~18:1
- Dark mode: `--kc-text-2` (#a8adb5) on `--kc-bg` (#090a0c) -- ratio ~8.5:1
- Light mode: `--kc-text-1` (#1a1c1e) on `--kc-bg` (#f8f9fa) -- ratio ~17:1
- Light mode: `--kc-accent-contrast` (#4a9c97) on `--kc-bg-elv` (#ffffff) -- ratio ~4.6:1

### Alternatives Considered

- **WCAG AAA:** Too restrictive; rules out the teal accent family entirely on white backgrounds.
- **No formal standard:** Unacceptable -- introduces unverifiable accessibility regressions.

### Consequences

Color additions or modifications to the palette require contrast verification before merge. The `--kc-accent-contrast` token exists specifically as the WCAG-AA-compliant variant of the teal accent for use on light elevated surfaces.

---

## ADR-004: VitePress as Primary Downstream Consumer

**Status:** Accepted  
**Date:** 2026-03-26

### Context

The majority of Phenotype documentation projects use VitePress. VitePress has its own CSS variable system (`--vp-c-*`) that must be remapped for custom theming. Consumers should not need to understand VitePress internals to apply the Keycap theme.

### Decision

Provide `css/vitepress-theme.css` as a dedicated first-class export that:
1. Composes `keycap-palette.css` and `components.css` via `@import`
2. Remaps all relevant `--vp-*` variables to `--kc-*` equivalents
3. Applies targeted component-zone overrides (nav, sidebar, hero, code blocks, feature cards)

Also provide `dist/vitepress.js` with typed `vitepressConfig` and `vitepressMarkdownTheme` exports for the `defineConfig()` entry point in `.vitepress/config.ts`.

### Alternatives Considered

- **Generic theme only (no VitePress mapping):** VitePress users would need 20+ manual variable overrides to achieve consistent theming.
- **VitePress Vue component overrides:** More powerful but requires Vue runtime; breaks non-Vue consumers; increases bundle size.

### Consequences

VitePress documentation sites get one-import theming. Other frameworks use the generic `css/keycap-palette.css` and `css/components.css` exports and map to their own variable systems.

---

## ADR-005: bun as Package Manager; TypeScript + oxlint as Toolchain

**Status:** Accepted  
**Date:** 2026-03-26

### Context

The repository needs a fast, consistent build and lint toolchain. The Phenotype ecosystem prefers bleeding-edge, OSS-first tooling.

### Decision

- **Package manager:** `bun` (detected via `bun.lock` lockfile, declared in `package.json` `packageManager`)
- **Build:** `tsc` (TypeScript compiler) + `cp -r css dist/css` shell step
- **Linter:** `oxlint` (Rust-based, fast, drop-in ESLint alternative)
- **Formatter:** `oxfmt`
- **Devdeps only:** `oxlint`, `typescript`, `vitepress`, `vue` -- zero runtime dependencies

### Alternatives Considered

- **npm/pnpm:** Slower; less aligned with ecosystem bleeding-edge preference.
- **ESLint + Prettier:** Heavier, slower than oxlint + oxfmt.
- **Rollup/esbuild bundler:** Overkill for a token library with two tiny TS source files.

### Consequences

The build output is bare TypeScript-compiled JS + copied CSS files. No bundler-specific artifacts. Distribution via `files` array in `package.json` (css/, dist/, tokens/).

---

## ADR-006: OKLCH Color Space Evaluation for Future Migration

**Status:** Proposed  
**Date:** 2026-04-04

### Context

Current Keycap palette uses sRGB hex values (#7ebab5, #090a0c). Research in `SOTA.md` indicates that OKLCH provides superior perceptual uniformity for color systems. Safari 16.2+, Chrome/Edge 111+, and Firefox 128+ now support CSS Color Module Level 4 with `oklch()` function.

### Decision

**Deferred adoption** with evaluation criteria. phenoDesign will maintain current hex values for backward compatibility but will evaluate OKLCH migration when:
1. Browser support reaches 95%+ of target user base
2. CSS Color Module Level 4 is Candidate Recommendation
3. Migration tooling exists for automated hex→OKLCH conversion
4. W3C DTCG specification adds native OKLCH type support

### Research Findings

| Color Space | Perceptual Uniformity | Browser Support | phenoDesign Fit |
|-------------|----------------------|-----------------|-----------------|
| sRGB/Hex | Poor | Universal | Current |
| HSL | Poor | Universal | Not better than hex |
| LCH | Good | 85%+ | Good intermediate |
| OKLCH | Excellent | 85%+ | **Target future** |

OKLCH benefits for design systems:
- Predictable lightness adjustments across hues
- Better color interpolation (no muddy mid-tones)
- Consistent perceptual brightness
- Automatic accessibility-compliant palette generation

### Alternatives Considered

- **Immediate migration to OKLCH:** Rejected due to incomplete browser support and breaking change for consumers.
- **Dual-format tokens (hex + OKLCH):** Rejected due to complexity and token bloat.
- **Stay with hex indefinitely:** Accepted for now, but limits future color system capabilities.

### Consequences

- Current hex values remain stable
- Future major version (v2.0) may introduce OKLCH
- Documentation will prepare consumers for potential migration
- Contrast calculations use current sRGB math (sufficient for WCAG AA)

---

## ADR-007: Fluid Typography System for Future Enhancement

**Status:** Proposed  
**Date:** 2026-04-04

### Context

Current typography uses fixed sizes (16px base). Research indicates fluid typography using CSS `clamp()` provides superior responsive behavior without media query breakpoints. All modern browsers support `clamp()` (Chrome 79+, Firefox 75+, Safari 13.1+).

### Decision

**Phase 2 enhancement.** phenoDesign will implement fluid typography as an optional feature in a future minor version:

<!-- stripped fenced code block: css -->

### Implementation Plan

1. **Design Phase:** Define fluid scale breakpoints (320px - 1920px)
2. **Token Addition:** Add `--kc-text-*-fluid` tokens alongside existing fixed tokens
3. **VitePress Integration:** Map fluid tokens to VP typography variables
4. **Documentation:** Provide migration guide for consumers

### Research Findings

Fluid typography benefits:
- Smooth scaling without breakpoint jumps
- Respects user font size preferences
- Reduces media query complexity
- Better reading experience across device sizes

### Alternatives Considered

- **Fixed typography only:** Current state; limits responsive design quality.
- **Breakpoint-based responsive typography:** Requires many media queries; less elegant than fluid.
- **CSS locks technique:** Pre-clamp() technique; now obsolete with native clamp() support.

### Consequences

- Existing fixed typography remains default (no breaking change)
- Fluid typography available as progressive enhancement
- Consumers can opt-in via token selection
- Future major version may make fluid the default

---

## ADR-008: Package Export Strategy for Subpath Imports

**Status:** Accepted  
**Date:** 2026-04-04

### Context

phenoDesign exports multiple artifacts: CSS files, JS modules, and JSON tokens. Node.js package.json `exports` field provides precise control over which files are importable and how they're resolved. Clear export strategy prevents consumers from importing internal files and enables clean public API.

### Decision

Explicit `exports` map with conditional types:

<!-- stripped fenced code block: json -->

### Export Categories

1. **JS API:** `.`, `./tokens`, `./vitepress` - Full conditional exports with types
2. **CSS Assets:** `./css/*` - Direct CSS file access
3. **Token Assets:** `./tokens/*` - Raw JSON token access

### Alternatives Considered

- **No exports field (main/module only):** Allows importing any file; no clear public API.
- **Wildcard exports only:** Lacks type support; less precise control.
- **Directory exports:** `./css/` style exports; less explicit about available files.

### Consequences

- Consumers have clear, documented import paths
- TypeScript resolves types correctly for all entry points
- Internal files cannot be accidentally imported
- Future additions follow established pattern

---

## ADR-009: Dark Mode Implementation Strategy

**Status:** Accepted  
**Date:** 2026-04-04

### Context

phenoDesign must support both light and dark modes. Three implementation strategies exist: CSS custom properties with media queries, class-based toggling, and data-attribute based toggling.

### Decision

**Hybrid approach** supporting all three methods:

1. **Light mode:** Default (no class/attribute needed)
2. **Dark mode class:** `.dark` selector for explicit opt-in
3. **Dark mode data attribute:** `[data-theme="dark"]` for JavaScript frameworks
4. **System preference:** `@media (prefers-color-scheme: dark)` for automatic detection

<!-- stripped fenced code block: css -->

### Research Findings

| Method | Pros | Cons | Use Case |
|--------|------|------|----------|
| Media query only | Zero JS | No manual toggle | Static sites |
| Class-based | Easy toggle | Requires JS | React/Vue apps |
| Data attribute | Framework idiomatic | Requires JS | Framework apps |
| Hybrid | Maximum flexibility | Slightly more CSS | phenoDesign |

### Alternatives Considered

- **Media query only:** Limits manual theme control.
- **Class only:** Requires adding class to root element; less flexible.
- **Data attribute only:** Some frameworks prefer class-based approach.

### Consequences

- Works with any framework or vanilla HTML
- Respects user system preferences by default
- Allows explicit theme control when needed
- CSS is slightly larger but negligible (~200 bytes)

---

## ADR-010: Documentation Site as Integration Test

**Status:** Accepted  
**Date:** 2026-04-04

### Context

phenoDesign needs continuous verification that tokens, CSS, and VitePress integration work correctly. A live documentation site serves dual purpose: user documentation and integration testing.

### Decision

**docs/ folder as integration test suite.** The VitePress documentation site in `docs/` serves as:

1. **User Documentation:** Explains tokens, components, integration
2. **Visual Regression Test:** Shows all tokens and components rendered
3. **Integration Test:** Verifies VitePress theme CSS works correctly
4. **CI Gate:** Build must pass before PR merge

### Test Strategy

<!-- stripped fenced code block: yaml -->

### Documentation Structure

```
docs/
├── .vitepress/          # VitePress config using phenoDesign
├── tokens/              # Token documentation
├── components/          # Component usage examples
├── reference/           # API reference
└── index.md             # Homepage
```

### Alternatives Considered

- **Separate test site:** Duplicates configuration; separate maintenance burden.
- **Storybook:** Heavy dependency; overkill for CSS-only components.
- **No live docs:** Relies on README only; insufficient for design system.

### Consequences

- Documentation always reflects current implementation
- Breaking changes break docs build (visible in CI)
- Dogfooding ensures VitePress integration quality
- Additional CI time for docs build (~10 seconds)

---

**End of ADR Document**

*Architecture Decision Records are living documents. Proposed ADRs become Accepted after team review and implementation.*
