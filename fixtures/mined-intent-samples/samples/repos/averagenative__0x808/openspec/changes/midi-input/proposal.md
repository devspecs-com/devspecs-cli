## Why

The standalone frontends (ImGui and GTK) have no way to receive MIDI input from external controllers or keyboards. Users with USB MIDI keyboards can only play notes via the on-screen virtual keyboard or QWERTY mapping. The plugin (VST3/CLAP) already receives MIDI from the DAW via CPLUG, but standalone users are locked out. Additionally, the sequencer has no note-off system — it triggers synth voices but never releases them based on step length, forcing all presets to use sustain=0 as a workaround. This prevents future presets from using held/sustained sounds.

## What Changes

- **RtMidi integration**: Vendor RtMidi as a cross-platform MIDI input library (ALSA on Linux, WinMM on Windows, CoreMIDI on macOS). Single header+source, permissive license.
- **MIDI device selector**: Add MIDI input device dropdown to the settings panel in both ImGui and GTK frontends. Support multiple devices, auto-detect first available on startup.
- **MIDI → synth routing**: MIDI note-on pushes `CMD_TRIGGER_NOTE` to the engine's lock-free command queue. MIDI note-off pushes `CMD_RELEASE_NOTE`. Engine processes these in the audio thread and calls `synth_trigger()` / envelope release.
- **Sequencer note-off**: Add step-length-based note release to the sequencer. Track active synth voices per track, and when a note's step length expires, enter the release phase. This enables future presets with sustain > 0.
- **MIDI learn** (stretch): Allow users to map MIDI CC to engine parameters (BPM, volume, etc.)

## Capabilities

### New Capabilities
- `midi-device-input`: RtMidi-based MIDI device enumeration, selection, and real-time note input for standalone frontends
- `sequencer-note-off`: Step-length-based synth voice release in the sequencer, enabling sustained presets

### Modified Capabilities

## Impact

- **Dependencies**: Add RtMidi (vendored in `deps/rtmidi/`) — C++ library with C-compatible callback API
- **Engine** (`src/engine/`): New `midi_input.c` for device management, extend command queue processing for `CMD_TRIGGER_NOTE` / `CMD_RELEASE_NOTE`, add note-off tracking to sequencer
- **Settings panel** (`src/gui/settings_panel.cpp`, `src/gui_gtk/gtk_settings.c`): Add MIDI device dropdown section
- **Audio init** (`main_gui.c`, `main_gtk.c`): Initialize RtMidi alongside SDL2 audio, start MIDI listener thread
- **CMakeLists.txt**: Add RtMidi source, link platform MIDI backends (libasound on Linux, winmm on Windows)
- **Plugin**: No changes — already receives MIDI from DAW
