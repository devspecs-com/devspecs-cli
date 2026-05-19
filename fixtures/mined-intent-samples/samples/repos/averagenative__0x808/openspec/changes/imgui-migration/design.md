## Context

The 0x808 sequencer uses a 3-layer architecture: Engine (C99 DSP) → GUI (Nuklear immediate-mode) → Host wrappers (standalone + plugin). The GUI layer has ~6,400 lines across 13 C files using ~790 Nuklear API calls.

Nuklear's window management has proven unreliable — the toolbar fails to render due to its auto-focus/ROM mechanism, and multiple workaround attempts (two-pass render, NK_WINDOW_NO_INPUT, draw-order manipulation, set_focus) have all failed. The project needs a GUI framework with proper panel/z-order control.

Current GUI components: toolbar, drum grid, piano roll, synth editor, virtual keyboard, sample browser, mixer view, knobs, arrangement, pattern presets, export dialog, theme system.

## Goals / Non-Goals

**Goals:**
- Replace Nuklear with Dear ImGui for all GUI rendering
- Fix toolbar rendering — always visible, no z-order hacks
- Maintain all existing GUI functionality (every button, slider, knob, grid cell)
- Keep engine layer as pure C99 — only GUI layer becomes C++
- Both standalone (0x808.exe) and plugin (VST3/CLAP) builds work
- Windows cross-compilation continues to work via MinGW
- Add overdrive, fuzz, and chorus effects to the engine

**Non-Goals:**
- No engine refactoring — DSP code stays as-is
- No new GUI features beyond what currently exists (except effects)
- No ImGui docking/multi-viewport — simple fixed-layout panels like current design
- No font changes or visual redesign — match current dark theme appearance
- No changes to audio threading, project save/load, or export

## Decisions

### Decision 1: Dear ImGui over alternatives

**Choice**: Dear ImGui (C++17, MIT license)

**Alternatives considered**:
- **Qt**: Too heavy, forces framework adoption, licensing complexity
- **JUCE**: Audio-focused but replaces entire architecture, GPL/commercial license
- **Staying with Nuklear**: Proven unworkable for overlapping window z-order
- **Custom retained-mode**: Too much effort for the problem at hand

**Rationale**: ImGui is the closest 1:1 replacement for Nuklear — same immediate-mode paradigm, same SDL2+OpenGL backend, but with reliable window/panel management. The porting effort is mechanical (API translation), not architectural.

### Decision 2: Source inclusion (not system library)

**Choice**: Vendor ImGui source files into `deps/imgui/` (same approach as Nuklear)

**Rationale**: Ensures reproducible builds, no system dependency, works with MinGW cross-compilation. ImGui is designed to be compiled as part of your project.

Files to vendor:
- `imgui.cpp`, `imgui.h`, `imgui_demo.cpp`, `imgui_draw.cpp`, `imgui_tables.cpp`, `imgui_widgets.cpp`, `imgui_internal.h`
- `backends/imgui_impl_sdl2.cpp`, `backends/imgui_impl_sdl2.h`
- `backends/imgui_impl_opengl3.cpp`, `backends/imgui_impl_opengl3.h`

### Decision 3: Fixed panel layout (not docking)

**Choice**: Use `ImGui::SetNextWindowPos` / `ImGui::SetNextWindowSize` with `ImGuiCond_Always` to create fixed panels, not ImGui's docking system.

**Rationale**: The current layout is fixed regions (toolbar at top, drum grid in middle, panels at bottom). This maps directly to positioned ImGui windows. Docking adds complexity we don't need — users don't rearrange panels.

### Decision 4: File-by-file port preserving component structure

**Choice**: Port each `src/gui/*.c` → `src/gui/*.cpp` as a direct translation, preserving the same public API signatures (with `extern "C"` where needed by the host layer).

**Rationale**: Minimizes risk. Each component can be ported and tested independently. The `*_draw()` function signatures change only in that they no longer take `struct nk_context*` (ImGui uses global state via `ImGui::GetIO()`).

### Decision 5: extern "C" guards on engine headers

**Choice**: Add `#ifdef __cplusplus extern "C" { #endif` guards to all engine and core headers.

**Rationale**: C++ GUI files must include C engine headers. The guards ensure C linkage. This is a one-time mechanical change with no runtime impact.

### Decision 6: Custom knob widget via ImGui draw list

**Choice**: Reimplement `knobs.c` using `ImGui::GetWindowDrawList()` for arc/circle rendering, with `ImGui::InvisibleButton()` for hit detection and `ImGui::GetIO().MouseDelta` for drag.

**Rationale**: ImGui doesn't have a built-in knob widget, but its draw list API is more capable than Nuklear's canvas API. The knob implementation pattern (invisible button + custom drawing) is well-established in ImGui codebases.

### Decision 7: New effects in engine (C99, decoupled from GUI)

**Choice**: Add overdrive, fuzz, and chorus as new effect types in `src/engine/effects.c`, following the existing `sq_effect_type_t` enum pattern.

**Rationale**: These are engine-level DSP changes that don't depend on the GUI framework. They can be implemented before, during, or after the ImGui migration.

## Risks / Trade-offs

- **Risk: Large diff** — ~6,400 lines of GUI code change at once → Mitigation: Port component-by-component with build verification at each step. Keep engine untouched.
- **Risk: C++ in a C project** — Mixed compilation can cause linker issues → Mitigation: Only GUI files are C++. Engine stays C99. `extern "C"` guards at boundaries. CMake handles mixed compilation natively.
- **Risk: Visual regression** — ImGui's default style differs from Nuklear's dark theme → Mitigation: Apply custom ImGui style colors to match current theme (table of ~25 color values).
- **Risk: MinGW C++ support** — Cross-compilation with C++ → Mitigation: MinGW fully supports C++. Update toolchain file to use `x86_64-w64-mingw32-g++` for .cpp files.
- **Risk: Plugin GUI embedding** — ImGui in a DAW host window → Mitigation: ImGui's SDL2 backend already handles `SDL_CreateWindowFrom()`. Same reparenting approach as current plugin_gui.c.
- **Trade-off: ImGui global state** — ImGui uses a single global context vs Nuklear's explicit context pointer → Acceptable for our single-window application. Plugin builds may need `ImGui::SetCurrentContext()` if multiple instances exist.
