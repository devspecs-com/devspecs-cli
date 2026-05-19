## 1. Vendor RtMidi

- [x] 1.1 Download RtMidi source (RtMidi.h + RtMidi.cpp) into deps/rtmidi/
- [x] 1.2 Add RtMidi to CMakeLists.txt as a static library, link platform backends (libasound on Linux, winmm on Windows, CoreMIDI+CoreAudio on macOS)
- [x] 1.3 Verify RtMidi builds on Linux and Windows cross-compile

## 2. C Wrapper (sq_midi)

- [x] 2.1 Create `src/engine/sq_midi.h` — C API: init, shutdown, get_port_count, get_port_name, open_port, close_port, set_callback
- [x] 2.2 Create `src/engine/sq_midi.cpp` — implements C API wrapping RtMidiIn (uses built-in rtmidi_c.h C API)
- [x] 2.3 MIDI callback parses note-on (0x90) and note-off (0x80) messages, pushes CMD_TRIGGER_NOTE / CMD_RELEASE_NOTE to engine command queue
- [x] 2.4 Add sq_midi to CMakeLists.txt (links against rtmidi library)

## 3. Engine Command Queue Integration

- [x] 3.1 Verify CMD_TRIGGER_NOTE and CMD_RELEASE_NOTE are defined in command_queue.h with correct fields (preset, note, velocity)
- [x] 3.2 Add command processing for CMD_TRIGGER_NOTE in sq_engine_process() — calls synth_trigger()
- [x] 3.3 Add command processing for CMD_RELEASE_NOTE in sq_engine_process() — finds matching voice by note/frequency and sets envelope to ENV_RELEASE
- [x] 3.4 Add MIDI config to sq_app_t — midi_device_name, midi_port_index

## 4. Sequencer Note-Off

- [x] 4.1 Add sq_active_note_t tracking array to sq_engine_t — track index, voice index, remaining samples
- [x] 4.2 When sequencer triggers a synth voice, record it in the active note tracker with duration = step.length * samples_per_step
- [x] 4.3 In sq_engine_process(), decrement remaining samples for all active notes; release voice when remaining hits zero
- [x] 4.4 On transport stop, release all tracked notes immediately
- [x] 4.5 Skip note-off tracking for steps with length = 0 (one-shot behavior, current default)
- [x] 4.6 Test: add a synth preset with sustain > 0, verify it releases after step length expires

## 5. Settings Panel — MIDI Section (ImGui)

- [x] 5.1 Add MIDI section to settings_panel.cpp — device dropdown using sq_midi_get_port_count/get_port_name
- [x] 5.2 "None" option to disable MIDI input
- [x] 5.3 Selecting a device calls sq_midi_open_port(), deselecting calls sq_midi_close_port()
- [x] 5.4 Show current MIDI status (connected / no device / error)

## 6. Settings Panel — MIDI Section (GTK)

- [x] 6.1 Add MIDI section to gtk_settings.c — GtkDropDown with MIDI port names
- [x] 6.2 "None" option, same open/close behavior as ImGui
- [x] 6.3 Refresh button to re-enumerate MIDI devices

## 7. Standalone Integration

- [x] 7.1 Initialize sq_midi in main_gui.c after audio init, auto-detect first available device
- [x] 7.2 Initialize sq_midi in main_gtk.c after audio init, same auto-detect
- [x] 7.3 Pass engine command queue pointer to sq_midi so callback can push commands
- [x] 7.4 Shutdown sq_midi on application exit in both frontends
- [x] 7.5 Exclude MIDI init from plugin builds — plugins don't link sq_midi

## 8. Testing

- [x] 8.1 Unit test: sq_midi_get_port_count returns >= 0 without crash (graceful when no ALSA/WinMM)
- [x] 8.2 Unit test: CMD_TRIGGER_NOTE / CMD_RELEASE_NOTE round-trip through command queue
- [x] 8.3 Unit test: sequencer note-off — trigger synth with length=4, verify voice enters release after 4 steps
- [x] 8.4 Integration test: sequencer note-off with a sustain>0 preset, verify audio decays after step length
- [x] 8.5 Verify plugin builds still compile without RtMidi dependency

## 9. Bug Fixes (discovered during implementation)

- [x] 9.1 REC button click registration: use stable ImGui ID (###rec_btn) and fixed width to prevent missed clicks when label changes every frame
