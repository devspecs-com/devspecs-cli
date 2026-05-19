## Why

Nuklear's immediate-mode window management has fundamental z-order and rendering bugs that cannot be worked around. After 5+ attempts to fix toolbar rendering (two-pass rendering, NK_WINDOW_NO_INPUT, set_focus reordering, ROM flag management, draw-order changes), the toolbar still fails to render on load. The root cause is Nuklear's auto-focus mechanism setting NK_WINDOW_ROM on non-active windows, which suppresses rendering despite being documented as input-only. This is a dead end — the project needs a GUI framework with reliable window/panel management.

Dear ImGui is the industry-standard immediate-mode GUI for SDL2+OpenGL applications. It has proper docking, z-order control, and is battle-tested in music/audio software. The engine layer stays pure C99; only the GUI layer (Layer 2) becomes C++.

## What Changes

- **BREAKING**: Remove Nuklear dependency (`deps/nuklear.h`, `deps/nuklear_sdl_gl3.h`)
- **BREAKING**: All `src/gui/*.c` files rewritten as `src/gui/*.cpp` using Dear ImGui API
- **BREAKING**: `src/plugin/plugin_gui.c` rewritten as `src/plugin/plugin_gui.cpp`
- Add Dear ImGui as a dependency (source files in `deps/imgui/`)
- Add ImGui SDL2+OpenGL3 backend files
- Update `CMakeLists.txt` for mixed C/C++ compilation
- All engine headers (`src/engine/*.h`, `src/core/*.h`) get `extern "C"` guards
- Toolbar becomes a persistent top bar (no window management tricks needed)
- Add new audio effects to the engine: overdrive, fuzz, chorus (C99, separate from GUI migration)

## Capabilities

### New Capabilities
- `imgui-gui-framework`: Dear ImGui integration with SDL2+OpenGL3 backend, replacing Nuklear across standalone and plugin builds
- `imgui-toolbar`: Fixed toolbar panel using ImGui — always visible, no z-order issues
- `imgui-drum-grid`: Drum grid component ported to ImGui with click-drag, right-click popup, step editing
- `imgui-piano-roll`: Piano roll component ported to ImGui with note placement/deletion
- `imgui-synth-editor`: Synth editor ported to ImGui with knobs, ADSR visualization, FM algorithm display, scroll-wheel combo cycling
- `imgui-mixer-browser`: Mixer view, sample browser, and supporting panels ported to ImGui
- `guitar-effects`: New engine effects — overdrive, fuzz, chorus (C99 DSP, no GUI dependency)

### Modified Capabilities

## Impact

- **Build system**: CMakeLists.txt needs C++ compiler for GUI layer, C99 stays for engine
- **Dependencies**: Remove nuklear.h/nuklear_sdl_gl3.h, add imgui/*.cpp + imgui/backends/imgui_impl_sdl2.cpp + imgui_impl_opengl3.cpp
- **API surface**: All `*_draw(struct nk_context*, ...)` signatures change to ImGui-style (no context pointer needed — ImGui uses global state)
- **Plugin builds**: VST3/CLAP plugin GUI wrapper changes from C to C++
- **Headers**: ~20 engine/core headers need `extern "C"` wrappers for C++ inclusion
- **Cross-compile**: MinGW toolchain file needs C++ support (x86_64-w64-mingw32-g++)
- **Distribution**: No new runtime DLLs — ImGui compiles into the executable like Nuklear did
