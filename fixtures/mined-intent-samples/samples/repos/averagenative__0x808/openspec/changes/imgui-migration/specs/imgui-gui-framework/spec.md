## ADDED Requirements

### Requirement: ImGui dependency integration
The build system SHALL include Dear ImGui source files from `deps/imgui/` and compile them as part of GUI targets. ImGui SDL2+OpenGL3 backend files SHALL be included.

#### Scenario: Fresh build with ImGui
- **WHEN** running `cmake .. && make sequencer_gui` from `build/`
- **THEN** ImGui source files compile without errors and link into the executable

#### Scenario: Windows cross-compile with ImGui
- **WHEN** running `cmake .. -DCMAKE_TOOLCHAIN_FILE=../cmake/mingw-w64.cmake && make sequencer_gui` from `build_win/`
- **THEN** ImGui compiles with MinGW C++ compiler and produces `0x808.exe`

### Requirement: Nuklear removal
The build system SHALL NOT include Nuklear headers or source files. All references to `nuklear.h` and `nuklear_sdl_gl3.h` SHALL be removed.

#### Scenario: No Nuklear references after migration
- **WHEN** searching the compiled source files for `nk_` function calls
- **THEN** zero matches are found (excluding any retained comments about migration history)

### Requirement: Mixed C/C++ compilation
CMakeLists.txt SHALL support compiling engine files as C99 and GUI files as C++. Engine headers SHALL have `extern "C"` guards.

#### Scenario: Engine headers included from C++
- **WHEN** a `.cpp` GUI file includes `engine/engine.h`
- **THEN** compilation succeeds with no linkage errors

### Requirement: ImGui frame loop
The GUI SHALL initialize ImGui with SDL2+OpenGL3 backend, process SDL events through ImGui, render all windows in a single pass, and swap buffers.

#### Scenario: Application startup
- **WHEN** the application starts
- **THEN** an SDL2 window opens with ImGui rendering at ~60fps with VSync

### Requirement: Dark theme styling
ImGui SHALL be styled with a dark theme matching the current application appearance (dark backgrounds, light text, blue accent colors).

#### Scenario: Visual consistency
- **WHEN** the application renders
- **THEN** window backgrounds are dark gray (~35,35,38), text is light gray (~210,210,210), and active elements use blue accents (~100,180,255)
