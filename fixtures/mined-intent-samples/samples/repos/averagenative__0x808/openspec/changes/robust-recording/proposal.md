## Why

The current recording system buffers all audio in RAM (capped at 10 minutes / 500 MB), saves to a hardcoded `recording.wav` that silently overwrites previous recordings, and truncates without warning when the buffer fills. Users have no control over save location or filename, and cannot record sessions longer than ~10 minutes. This makes recording unreliable for real use — a user who records a 2-3 hour jam session will lose everything past the first 10 minutes with no indication anything went wrong.

## What Changes

- **Streaming WAV writer**: Replace in-memory buffer with a streaming writer that flushes audio chunks directly to disk during recording, enabling unlimited recording duration bounded only by available disk space
- **File save controls**: Let users choose an output directory and filename for recordings, with a sensible default (`~/Music/0x808/` or equivalent)
- **Auto-incrementing filenames**: Automatically number recordings (`recording_001.wav`, `recording_002.wav`, ...) so previous recordings are never overwritten
- **Recording status display**: Show elapsed recording time, file size, and disk space in the toolbar during recording
- **Disk-full handling**: Monitor available disk space during recording and warn the user before disk runs out, gracefully finalizing the WAV file if space is exhausted
- **Recording format options**: Allow users to select bit depth (16/24/32-bit) for recordings, matching the existing export format options

## Capabilities

### New Capabilities
- `streaming-recorder`: Engine-level streaming WAV writer that writes audio chunks to disk in real-time during recording, replacing the in-memory buffer approach
- `recording-ui`: User-facing recording controls — output directory/filename selection, format options, elapsed time display, disk space monitoring, and auto-incrementing filenames

### Modified Capabilities

## Impact

- **Engine layer** (`src/engine/`): New streaming writer in `export.c`/`export.h`, modified recording state in `engine.c`/`engine.h` — replaces `rec_buffer`/`rec_frames`/`rec_capacity` with streaming file handle and chunk writer
- **ImGui frontend** (`src/gui/`): Recording controls in toolbar, file/folder selection dialog, recording status overlay
- **GTK frontend** (`src/gui_gtk/`): Same recording controls using native GTK file chooser and widgets
- **sq_app controller** (`src/app/`): Shared recording state (output dir, filename pattern, format preference) to keep frontends in sync
- **Dependencies**: None new — dr_wav already supports streaming writes
