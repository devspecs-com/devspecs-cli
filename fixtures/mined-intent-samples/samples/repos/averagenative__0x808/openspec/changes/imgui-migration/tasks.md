## 1. Dependencies & Build System

- [x] 1.1 Download Dear ImGui source files and vendor into `deps/imgui/` (core + SDL2+OpenGL3 backends)
- [x] 1.2 Add `extern "C"` guards to all engine/core headers (`src/engine/*.h`, `src/core/*.h`, `src/formats/*.h`)
- [x] 1.3 Update `CMakeLists.txt`: add ImGui source files, enable C++ for GUI targets, remove Nuklear references
- [x] 1.4 Update `cmake/mingw-w64.cmake` toolchain for C++ cross-compilation (already had CXX compiler set)
- [x] 1.5 Verify both Linux native and Windows MinGW builds compile with ImGui

## 2. ImGui Bootstrap & Frame Loop

- [x] 2.1 Create `src/gui/gui.cpp` — ImGui init (SDL2+OpenGL3 backend), font loading, dark theme style setup
- [x] 2.2 Implement `gui_frame()` — SDL event polling via ImGui, glClear, ImGui::NewFrame/Render, swap buffers
- [x] 2.3 Implement `gui_shutdown()` — ImGui cleanup, SDL teardown
- [x] 2.4 Create `src/gui/theme.cpp` — dark/light theme color tables for ImGui (matching current Nuklear colors)
- [x] 2.5 Verify standalone app launches with ImGui showing an empty dark window

## 3. Toolbar (Fixed Panel)

- [x] 3.1 Implement toolbar as a fixed ImGui window (pos=0,0, size=full_width x 80, no collapse/resize/move)
- [x] 3.2 Add transport controls: PLAY/STOP button
- [x] 3.3 Add BPM slider (40-300), Swing slider (0-100%), Volume slider (0-100%)
- [x] 3.4 Add panel toggle buttons: EXPORT, PRESETS, PIANO, PAT/SONG/PERF, FX, BROWSE, EXIT with active highlights
- [x] 3.5 Add status message display with auto-dismiss timer
- [x] 3.6 Verify toolbar is always visible on first frame and stays visible during all interactions

## 4. Drum Grid

- [x] 4.1 Create `src/gui/drum_grid.cpp` — grid window with track rows and step columns
- [x] 4.2 Implement step cell rendering (colored rectangles for active/inactive, velocity-based opacity)
- [x] 4.3 Implement click-to-toggle and click-drag for multi-step editing
- [x] 4.4 Add track labels, type selector dropdown (sampler/synth), sample/preset selector
- [x] 4.5 Add per-track volume slider, pan control, mute/solo buttons
- [x] 4.6 Add right-click popup for step velocity/note editing
- [x] 4.7 Implement visual playhead (highlighted column during playback)
- [x] 4.8 Focus management for DrumGrid (no longer needed — ImGui handles z-order properly)

## 5. Knob Widget

- [x] 5.1 Create `src/gui/knobs.cpp` — custom rotary knob using ImGui::InvisibleButton + DrawList arc/circle rendering
- [x] 5.2 Implement vertical drag interaction with shift-drag for fine control
- [x] 5.3 Implement double-click to reset to default value
- [x] 5.4 Port `knob_float()`, `knob_mini()`, `knob_inline()` variants

## 6. Synth Editor

- [x] 6.1 Create `src/gui/synth_editor.cpp` — synth parameter panel with mode tabs (Subtractive/FM/Wavetable)
- [x] 6.2 Implement preset dropdown with scroll wheel cycling
- [x] 6.3 Implement mode dropdown with scroll wheel cycling
- [x] 6.4 Port oscillator, filter, and ADSR knob parameters
- [x] 6.5 Implement ADSR envelope curve visualization using ImGui DrawList
- [x] 6.6 Port FM algorithm diagram rendering
- [x] 6.7 Port wavetable waveform display

## 7. Piano Roll

- [x] 7.1 Create `src/gui/piano_roll.cpp` — note grid with keyboard reference
- [x] 7.2 Implement note rendering (filled rectangles per note with velocity coloring)
- [x] 7.3 Implement click to place/delete notes
- [x] 7.4 Implement playhead line during playback
- [x] 7.5 Add scrolling for note range and time range

## 8. Supporting Panels

- [x] 8.1 Create `src/gui/mixer_view.cpp` — per-track channel strips with volume, pan, mute, solo, effects
- [x] 8.2 Create `src/gui/sample_browser.cpp` — file list with sample preview and assignment
- [x] 8.3 Create `src/gui/virtual_keyboard.cpp` — 3-octave piano with click-to-play and octave shift
- [x] 8.4 Create `src/gui/arrangement.cpp` — pattern chain editor for SONG/PERFORM modes
- [x] 8.5 Create `src/gui/pattern_presets.cpp` — preset save/load dialog
- [x] 8.6 Create `src/gui/export_dialog.cpp` — WAV export settings and trigger

## 9. Keyboard Shortcuts & Input

- [x] 9.1 Port keyboard shortcut handling (Space=play, Escape=quit, Ctrl+S=save, Ctrl+O=load, etc.)
- [x] 9.2 Port QWERTY piano mapping (virtual_keyboard_key_event)
- [x] 9.3 Port pattern switching (1-9 keys), copy/paste (Ctrl+C/V), undo/redo (Ctrl+Z/Shift+Z)

## 10. Plugin GUI

- [x] 10.1 Create `src/plugin/plugin_gui.cpp` — ImGui init in embedded SDL window (host HWND reparenting)
- [x] 10.2 Port render thread with ImGui frame loop (same components as standalone)
- [x] 10.3 Handle ImGui context per plugin instance (`ImGui::SetCurrentContext()`)
- [x] 10.4 Verify VST3 and CLAP plugin builds with ImGui GUI (Linux + Windows)

## 11. Cleanup & Removal

- [x] 11.1 Remove `deps/nuklear.h` and `deps/nuklear_sdl_gl3.h`
- [x] 11.2 Remove old `src/gui/*.c` files (replaced by `.cpp` versions, kept `undo.c`)
- [x] 11.3 Remove old `src/plugin/plugin_gui.c`
- [x] 11.4 `gl3_loader.h` still needed by plugin_gui.cpp for Windows GL loading
- [x] 11.5 No remaining Nuklear references in src/ (verified with grep)

## 12. Guitar Effects (Engine — C99)

- [x] 12.1 Add `EFFECT_OVERDRIVE` to `sq_effect_type_t` enum with drive, tone, mix parameters
- [x] 12.2 Implement overdrive DSP: soft-clipping saturation with tone filter
- [x] 12.3 Add `EFFECT_FUZZ` to `sq_effect_type_t` enum with gain, tone, mix parameters
- [x] 12.4 Implement fuzz DSP: hard-clipping distortion with tone shaping
- [x] 12.5 Add `EFFECT_CHORUS` to `sq_effect_type_t` enum with rate, depth, mix parameters
- [x] 12.6 Implement chorus DSP: modulated delay line with LFO
- [x] 12.7 Wire new effects into effect_init/effect_free/effect_process + tests pass

## 13. Integration Testing

- [x] 13.1 Linux build compiles all targets (standalone, VST3, CLAP)
- [x] 13.2 Windows cross-compile produces 0x808.exe (15MB, PE32+ x86-64)
- [x] 13.3 Plugin targets build (VST3 + CLAP shared libraries)
- [ ] 13.4 Verify all keyboard shortcuts work (requires GUI launch)
- [x] 13.5 All existing tests pass (engine, effects, synth, project, undo, etc.)
- [x] 13.6 Deployed exe to `C:\Users\Dan Michael\Desktop\0x808\`

## 14. Extract GUI Library (`libsq_gui`)

- [x] 14.1 Create `libsq_gui` static library target in CMakeLists.txt with all shared GUI .cpp files
- [x] 14.2 Move globals (`g_win_width`, `g_win_height`, `g_visual_step`, `g_selected_track`) into `gui_globals.cpp` in the library
- [x] 14.3 Remove duplicate GUI source lists from standalone and plugin targets — link `sq_gui` instead
- [ ] 14.4 Define a `sq_gui_host_t` interface (function pointers or callbacks) so the library doesn't depend on the host wrapper
- [ ] 14.5 Refactor `gui.cpp` into library init/frame/shutdown + standalone `main_gui.c` calls only the library API
- [ ] 14.6 Refactor `plugin_gui.cpp` to use the library API (init with ImGui context, frame, shutdown)
- [x] 14.7 Verify standalone, VST3, and CLAP all build and link against `sq_gui`
- [x] 14.8 Verify Windows cross-compile still works
- [ ] 14.9 Document the library API in `src/gui/gui.h` for future frontend ports (e.g., GTK, Qt, web)

## 15. GTK 4.0 Frontend (Deferred)

- [ ] 15.1 Create `src/gui_gtk/` directory structure for GTK 4.0 frontend
- [ ] 15.2 Add GTK 4.0 build target in CMakeLists.txt (optional, behind `BUILD_GTK` option)
- [ ] 15.3 Implement GTK 4.0 window creation and main loop wrapper
- [ ] 15.4 Port toolbar with GtkHeaderBar or GtkBox + GtkButton/GtkScale widgets
- [ ] 15.5 Implement drum grid using GtkDrawingArea with Cairo rendering
- [ ] 15.6 Implement piano roll using GtkDrawingArea with Cairo rendering
- [ ] 15.7 Port synth editor knobs (custom GtkWidget or GtkDrawingArea)
- [ ] 15.8 Port sample browser using GtkTreeView/GtkListView
- [ ] 15.9 Port mixer view, arrangement, virtual keyboard, pattern presets, export dialog
- [ ] 15.10 Wire GTK frontend to `sq_engine` via the same C API used by ImGui frontend
- [ ] 15.11 Verify GTK 4.0 frontend builds and runs on Linux
- [ ] 15.12 Test alongside ImGui frontend (both should be buildable from same source tree)
