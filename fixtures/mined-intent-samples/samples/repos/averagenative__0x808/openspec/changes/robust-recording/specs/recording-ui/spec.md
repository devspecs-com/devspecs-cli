## ADDED Requirements

### Requirement: Recording output directory configuration
The system SHALL allow users to configure the output directory for recordings, with a platform-appropriate default (e.g., `~/Music/0x808/` on Linux, `Music\0x808\` on Windows).

#### Scenario: Default output directory
- **WHEN** the user has not configured a recording directory
- **THEN** recordings are saved to the platform default music directory under an `0x808` subfolder, creating it if it does not exist

#### Scenario: User changes output directory
- **WHEN** the user selects a different output directory via the UI
- **THEN** subsequent recordings are saved to the new directory

#### Scenario: Configured directory persists
- **WHEN** the user sets an output directory and restarts the application
- **THEN** the configured directory is restored from project settings

### Requirement: Recording filename prefix
The system SHALL allow users to set a filename prefix for recordings, defaulting to `recording`.

#### Scenario: Default prefix
- **WHEN** the user has not set a custom prefix
- **THEN** files are named `recording_001.wav`, `recording_002.wav`, etc.

#### Scenario: Custom prefix
- **WHEN** the user sets the prefix to `jam-session`
- **THEN** files are named `jam-session_001.wav`, `jam-session_002.wav`, etc.

### Requirement: Recording elapsed time display
The system SHALL display the elapsed recording time in the toolbar while recording is active.

#### Scenario: Time display during recording
- **WHEN** recording is in progress
- **THEN** the toolbar shows elapsed time in `MM:SS` format (or `HH:MM:SS` if over 60 minutes), updating every second

#### Scenario: Time display when not recording
- **WHEN** recording is not active
- **THEN** no recording time is displayed in the toolbar

### Requirement: Recording file size display
The system SHALL display the current recording file size during recording.

#### Scenario: File size shown during recording
- **WHEN** recording is in progress
- **THEN** the toolbar shows the approximate file size in human-readable format (KB, MB, GB)

### Requirement: Recording bit depth selection
The system SHALL provide a UI control for selecting recording bit depth (16/24/32-bit) before starting a recording.

#### Scenario: Change bit depth before recording
- **WHEN** the user selects 24-bit before starting recording
- **THEN** the recording is captured at 24-bit depth

#### Scenario: Bit depth locked during recording
- **WHEN** recording is in progress
- **THEN** the bit depth selector is disabled (cannot change mid-recording)

### Requirement: Disk space warning
The system SHALL warn the user when available disk space drops below a threshold during recording.

#### Scenario: Low disk space warning
- **WHEN** available disk space drops below 500 MB during recording
- **THEN** a warning is displayed to the user indicating low disk space

#### Scenario: Recording stopped on disk full
- **WHEN** a write failure occurs due to disk full
- **THEN** recording stops automatically and a status message indicates the recording was stopped due to insufficient disk space, with the path to the saved partial file

### Requirement: Frontend parity
Both ImGui and GTK frontends SHALL implement identical recording controls and status displays.

#### Scenario: ImGui recording controls
- **WHEN** using the ImGui frontend
- **THEN** all recording controls (directory, prefix, bit depth, start/stop, status) are available

#### Scenario: GTK recording controls
- **WHEN** using the GTK frontend
- **THEN** all recording controls (directory, prefix, bit depth, start/stop, status) are available and functionally identical to ImGui
