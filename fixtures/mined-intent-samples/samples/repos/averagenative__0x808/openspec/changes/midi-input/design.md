## Context

The engine has a lock-free command queue (`sq_command_queue_t`) with `CMD_TRIGGER_NOTE` and `CMD_RELEASE_NOTE` already defined but unused. The plugin receives MIDI via CPLUG's `dequeueEvent()` and calls `synth_trigger()` directly in the audio process callback. The standalone frontends use SDL2 for audio (push-based threading) and have a settings panel with audio device selection. The sequencer (`sequencer.c`) triggers voices on step boundaries but has no mechanism to release them when `step.length` expires.

## Goals / Non-Goals

**Goals:**
- External MIDI keyboard input in standalone (ImGui + GTK)
- MIDI device enumeration and hot-selection in settings panel
- Note-on/note-off routed through the existing command queue
- Sequencer note-off based on step.length for synth tracks
- Cross-platform: Linux (ALSA), Windows (WinMM), macOS (CoreMIDI)

**Non-Goals:**
- MIDI output (sending MIDI to external devices)
- MIDI clock sync (syncing BPM to external clock) — future feature
- MIDI CC mapping to engine parameters — stretch goal, not required for v1.3.0
- Plugin MIDI changes (already working via CPLUG)
- MIDI recording/playback (recording MIDI events to a track)

## Decisions

### 1. RtMidi as the MIDI library

RtMidi is the de facto standard for cross-platform MIDI I/O. It's C++ but exposes a simple callback-based API that's easy to wrap in C. Single source file (~3K lines), permissive MIT license, zero external dependencies beyond OS APIs (ALSA, WinMM, CoreMIDI).

**Why over raw platform APIs:** Writing ALSA + WinMM + CoreMIDI from scratch is 3x the work for the same result. RtMidi is battle-tested and handles edge cases (device hot-plug, error recovery).

**Why over PortMidi:** PortMidi is older, less maintained, and has a more complex API. RtMidi is the modern standard.

### 2. C wrapper around RtMidi

Create a thin `sq_midi.h` / `sq_midi.cpp` wrapper that exposes a C API:
- `sq_midi_init()` / `sq_midi_shutdown()`
- `sq_midi_get_port_count()` / `sq_midi_get_port_name()`
- `sq_midi_open_port()` / `sq_midi_close_port()`
- `sq_midi_set_callback()` — registers a function called on every MIDI message

This keeps RtMidi's C++ isolated from the C engine/app code.

### 3. MIDI callback pushes to command queue

The RtMidi callback fires on a MIDI thread (not audio thread). It parses the MIDI message and pushes `CMD_TRIGGER_NOTE` or `CMD_RELEASE_NOTE` to the engine's existing lock-free command queue. The audio thread pops and processes these alongside other commands (BPM, volume, etc.). This is thread-safe with zero locking.

**Why not direct synth_trigger():** `synth_trigger()` modifies engine state that the audio thread reads. Calling it from the MIDI thread would be a data race. The command queue is the correct synchronization mechanism.

### 4. Sequencer note-off via voice tracking

Add a `sq_active_note_t` array to the engine that tracks which synth voices were triggered by which track/step, and their remaining duration in samples. Each audio buffer, decrement the remaining duration. When it hits zero, set the voice's envelope to `ENV_RELEASE`.

**Structure:**
```c
typedef struct {
    int      track;          /* which track triggered this */
    int      voice_index;    /* which synth voice */
    uint32_t remaining;      /* samples until note-off */
} sq_active_note_t;
```

This is processed in `sq_engine_process()` after the mixer, keeping it real-time safe (just integer decrements and comparisons).

### 5. MIDI device selection persists in sq_app

Add `midi_device_name[128]` and `midi_port_index` to `sq_audio_config_t` (which already holds audio device config). The settings panel populates a dropdown from `sq_midi_get_port_count()` / `sq_midi_get_port_name()`. Selecting a device calls `sq_midi_open_port()`.

## Risks / Trade-offs

- **RtMidi is C++** → Isolated in `sq_midi.cpp` with a C API wrapper. Only one .cpp file touches RtMidi. The engine stays pure C.
- **MIDI latency** → RtMidi callback → command queue → next audio buffer. Worst case is one audio buffer of latency (~11ms at 512 frames/44.1kHz). Acceptable for a step sequencer; competitive with most DAWs.
- **Device hot-plug** → RtMidi handles device enumeration at query time. If a device is unplugged mid-session, the callback stops firing. User re-opens settings and picks a new device. No crash.
- **Multiple MIDI devices** → Support opening one device at a time for simplicity. Multi-device is a future enhancement.
