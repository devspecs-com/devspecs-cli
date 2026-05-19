## ADDED Requirements

### Requirement: Streaming WAV writer
The engine SHALL provide an `sq_recorder` component that streams audio directly to a WAV file on disk during recording, writing each audio callback's output as PCM frames without accumulating a large in-memory buffer.

#### Scenario: Start recording opens file
- **WHEN** recording is started with a valid output path and bit depth
- **THEN** the system opens a new WAV file for streaming writes and begins capturing audio output each process cycle

#### Scenario: Audio written each callback
- **WHEN** the engine processes an audio buffer while recording is active
- **THEN** the processed stereo output is converted to the selected PCM bit depth and written to the open WAV file within the same callback

#### Scenario: Stop recording finalizes file
- **WHEN** recording is stopped
- **THEN** the WAV file header is finalized with correct data size and the file is closed, producing a valid WAV file

### Requirement: Unlimited recording duration
The system SHALL support recording durations limited only by available disk space, not by a fixed memory buffer.

#### Scenario: Record beyond previous 10-minute limit
- **WHEN** a user records for longer than 10 minutes
- **THEN** the recording continues without truncation or data loss as long as disk space is available

#### Scenario: Memory usage stays constant
- **WHEN** recording is in progress regardless of duration
- **THEN** the recorder's memory usage SHALL remain constant (no growing buffer), bounded by the audio callback buffer size

### Requirement: Disk-full handling
The system SHALL detect when a disk write fails or disk space is critically low and gracefully stop recording.

#### Scenario: Write failure during recording
- **WHEN** a WAV write returns fewer frames than requested (disk full or I/O error)
- **THEN** the system stops recording, finalizes the WAV file (making it valid up to the last successful write), and reports an error status

#### Scenario: Partial recording is valid
- **WHEN** recording stops due to disk-full condition
- **THEN** the saved WAV file SHALL be a valid, playable WAV file containing all audio up to the point of failure

### Requirement: Configurable bit depth
The system SHALL support recording in 16-bit, 24-bit, or 32-bit PCM WAV format.

#### Scenario: Record at each bit depth
- **WHEN** recording is started with bit depth set to 16, 24, or 32
- **THEN** the output WAV file contains PCM samples at the specified bit depth

### Requirement: Auto-incrementing filenames
The system SHALL automatically generate unique filenames by scanning the output directory and incrementing a numeric suffix to prevent overwriting previous recordings.

#### Scenario: First recording in empty directory
- **WHEN** recording starts and no previous recordings exist in the output directory
- **THEN** the file is named `{prefix}_001.wav`

#### Scenario: Subsequent recording with existing files
- **WHEN** recording starts and files `{prefix}_001.wav` through `{prefix}_005.wav` exist
- **THEN** the new file is named `{prefix}_006.wav`

#### Scenario: Gap in numbering
- **WHEN** recording starts and files `{prefix}_001.wav` and `{prefix}_003.wav` exist (002 deleted)
- **THEN** the new file is named `{prefix}_004.wav` (uses max + 1, does not fill gaps)
