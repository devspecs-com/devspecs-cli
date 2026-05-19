## Context

The current recording system in 0x808 pre-allocates a single contiguous float buffer in RAM (`rec_buffer`) sized for 10 minutes of stereo audio (~230 MB at 48 kHz). Audio from `sq_engine_process()` is memcpy'd into this buffer each frame. When recording stops, the entire buffer is written to `{base_dir}/recording.wav` as 16-bit PCM. There is no user control over filename or location, no indication of elapsed time, and the file is silently overwritten on each recording.

The engine already vendors `dr_wav` which supports streaming writes via `drwav_init_file_write()` and `drwav_write_pcm_frames()`. The export system (`export.c`) already handles WAV/MP3/FLAC writing but only in offline (non-realtime) mode.

## Goals / Non-Goals

**Goals:**
- Stream recorded audio directly to disk in real-time (no large RAM buffer)
- Let users configure output directory and filename pattern
- Auto-increment filenames to prevent overwrites
- Display recording elapsed time and file size in the toolbar
- Gracefully handle disk-full conditions
- Support 16/24/32-bit WAV recording

**Non-Goals:**
- Recording to MP3/FLAC in real-time (encoding overhead too high for audio thread)
- Multi-track recording (stems) — future feature
- Recording input audio (microphone) — this records the engine mix output only
- Post-recording format conversion — user can use the existing export system for that

## Decisions

### 1. Streaming WAV via dr_wav

Use `drwav_init_file_write()` to open a WAV file at recording start, then call `drwav_write_pcm_frames()` from the audio callback with each processed chunk. dr_wav handles header updates on close via `drwav_uninit()`.

**Why dr_wav over raw file I/O:** Already vendored, handles WAV header bookkeeping (data chunk size fixup on close), cross-platform, and supports 16/24/32-bit PCM natively.

**Alternative considered:** Ring buffer with a writer thread. More complex, but the audio callback would never block on I/O. Rejected because dr_wav writes are small (256-512 frames = 1-2 KB), and modern OS file I/O with write-back caching won't block for such small writes. If latency issues emerge, we can add a ring buffer later without changing the API.

### 2. File I/O from audio thread with small writes

The audio callback writes 256-512 frames per call (~1-2 KB of PCM data). On all target platforms (Linux ALSA/PulseAudio, Windows WASAPI), this hits the OS page cache and returns immediately — the kernel flushes to disk asynchronously. This avoids the complexity of a dedicated writer thread.

**Risk mitigation:** If a platform shows blocking behavior, the fallback is a lock-free ring buffer drained by a writer thread. The `sq_recorder` API abstracts this so the change would be internal only.

### 3. Auto-incrementing filenames with scan-on-start

At recording start, scan the output directory for files matching the pattern `{prefix}_{NNN}.wav` and pick `max(NNN) + 1`. Store the counter in `sq_recorder` state so subsequent recordings in the same session increment without re-scanning.

**Why scan over persistent counter:** A persistent counter can drift if users delete or rename files. Scanning is O(n) in directory entries but only runs once per recording start — negligible cost.

### 4. Recording state in sq_app, not engine

Move recording configuration (output dir, filename prefix, bit depth, auto-increment counter) into `sq_app` alongside other shared UI state. The engine gets a thin `sq_recorder` struct that holds the open `drwav` handle and frame counter. This keeps the engine focused on audio and lets both frontends share recording preferences.

### 5. WAV-only for streaming recording

Real-time MP3/FLAC encoding adds CPU load to the audio thread and risks underruns. WAV is lossless, zero-overhead, and users can convert after via the existing export dialog. If real-time compressed recording is wanted later, it would use the ring-buffer + encoder-thread architecture.

## Risks / Trade-offs

- **Disk I/O from audio thread** → Mitigated by small write sizes (1-2 KB) hitting OS page cache. Monitor for glitches on low-end hardware; ring buffer is the escape hatch.
- **WAV file corruption on crash** → If the app crashes mid-recording, the WAV header won't have the correct data size. Mitigation: periodically update the WAV header (every ~10 seconds) by seeking back and rewriting the size fields via `drwav_uninit()` + reopen, or accept that crash recovery requires a WAV repair tool. For v1, accept this risk — it matches behavior of most DAWs.
- **Disk full during recording** → Check `drwav_write_pcm_frames()` return value each write. If fewer frames written than requested, stop recording, finalize the file, and show a warning. The partial file will be valid WAV up to the last successful write.
- **Large files on FAT32** → FAT32 has a 4 GB file limit (~6.3 hours at 48kHz/16-bit stereo). Not a concern for most users but worth documenting. No special handling needed — dr_wav will fail on write and we'll catch it.
