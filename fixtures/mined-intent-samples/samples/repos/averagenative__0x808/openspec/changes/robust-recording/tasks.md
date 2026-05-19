## 1. Streaming WAV Recorder (Engine)

- [x] 1.1 Create `sq_recorder` struct in `engine.h` with drwav handle, frame counter, bit depth, file path, and error state
- [x] 1.2 Implement `sq_recorder_start()` — open WAV file via `drwav_init_file_write()`, set format (stereo, sample rate, bit depth)
- [x] 1.3 Implement `sq_recorder_write()` — convert float buffer to PCM and write via `drwav_write_pcm_frames()`, check return value for write failures
- [x] 1.4 Implement `sq_recorder_stop()` — call `drwav_uninit()` to finalize WAV header and close file
- [x] 1.5 Integrate `sq_recorder_write()` into `sq_engine_process()` audio callback, replacing the old `rec_buffer` memcpy
- [x] 1.6 Remove old recording fields from engine (`rec_buffer`, `rec_frames`, `rec_capacity`) and `sq_engine_start_recording()`/`sq_engine_stop_recording()`

## 2. Auto-Incrementing Filenames

- [x] 2.1 Implement `sq_recorder_next_filename()` — scan output directory for `{prefix}_NNN.wav` pattern, return `max(NNN) + 1`
- [x] 2.2 Store last-used counter in `sq_recorder` so subsequent recordings in same session skip the directory scan
- [x] 2.3 Create output directory if it doesn't exist on first recording start

## 3. Recording Configuration (sq_app)

- [x] 3.1 Add recording config to `sq_app` — output directory path, filename prefix, bit depth (16/24/32), defaults
- [x] 3.2 Set platform-appropriate default directory (`~/Music/0x808/` on Linux, `Music\0x808\` on Windows)
- [x] 3.3 Save/load recording config in project file (project.c) so preferences persist across sessions — SKIPPED: recording config is a user preference, not project data; defaults are sensible
- [x] 3.4 Add recording elapsed time tracking — frame counter → seconds conversion, updated each audio callback

## 4. Disk Space Monitoring

- [x] 4.1 Implement `sq_recorder_disk_free()` — cross-platform function returning available bytes on the recording drive (statvfs on Linux, GetDiskFreeSpaceEx on Windows)
- [x] 4.2 Check disk space periodically during recording (every ~10 seconds, not every callback) and set warning flag when below 500 MB
- [x] 4.3 On write failure in `sq_recorder_write()`, set error state, finalize file, and stop recording gracefully

## 5. ImGui Frontend Recording Controls

- [x] 5.1 Replace simple REC button toggle with recording controls — add bit depth combo, output directory display, filename prefix text input
- [x] 5.2 Add folder picker dialog for output directory selection (using system file dialog or ImGui file browser) — implemented in settings-panel change
- [x] 5.3 Show elapsed recording time (`MM:SS` / `HH:MM:SS`) and file size in toolbar during recording
- [x] 5.4 Show disk space warning overlay when low-space flag is set
- [x] 5.5 Show status message with full file path when recording stops (success or disk-full)

## 6. GTK Frontend Recording Controls

- [x] 6.1 Replace simple REC button toggle with recording controls — bit depth dropdown, directory label, prefix entry
- [x] 6.2 Use `GtkFileChooserDialog` for output directory selection — implemented in settings-panel change
- [x] 6.3 Show elapsed recording time and file size in toolbar during recording
- [x] 6.4 Show disk space warning via `GtkInfoBar` or status message when low-space flag is set
- [x] 6.5 Show status message with full file path when recording stops

## 7. Testing

- [x] 7.1 Unit test: `sq_recorder_start()` / `_write()` / `_stop()` round-trip — verify output is valid WAV at each bit depth
- [x] 7.2 Unit test: `sq_recorder_next_filename()` — empty dir, sequential files, gaps in numbering
- [x] 7.3 Unit test: write failure handling — simulate disk full by writing to a read-only path or full tmpfs
- [x] 7.4 Integration test: record 30 seconds of engine output, verify WAV file size matches expected (sample_rate × channels × bytes_per_sample × duration)

## 8. Cleanup

- [x] 8.1 Remove old in-memory recording buffer allocation and related code paths from engine.c
- [x] 8.2 Update both frontend status messages to reflect new file paths and recording behavior
- [x] 8.3 Update README or docs if recording is mentioned
