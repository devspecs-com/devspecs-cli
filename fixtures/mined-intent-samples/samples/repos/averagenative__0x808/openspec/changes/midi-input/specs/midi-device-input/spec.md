## ADDED Requirements

### Requirement: MIDI device enumeration
The system SHALL enumerate available MIDI input devices and present them in the settings panel.

#### Scenario: List available MIDI devices
- **WHEN** the settings panel is opened
- **THEN** a dropdown lists all available MIDI input ports plus a "None" option

#### Scenario: No MIDI devices available
- **WHEN** no MIDI input devices are connected
- **THEN** the dropdown shows only "None" and the system operates without MIDI input

### Requirement: MIDI device selection
The system SHALL allow users to select a MIDI input device and begin receiving note data.

#### Scenario: Select a MIDI device
- **WHEN** the user selects a MIDI device from the dropdown
- **THEN** the system opens the selected MIDI port and begins receiving MIDI messages

#### Scenario: Deselect MIDI device
- **WHEN** the user selects "None" from the MIDI dropdown
- **THEN** the system closes any open MIDI port and stops receiving MIDI messages

### Requirement: MIDI note-on triggers synth
The system SHALL trigger synth voices when MIDI note-on messages are received from the selected device.

#### Scenario: Play a note on MIDI keyboard
- **WHEN** a MIDI note-on message (status 0x90, velocity > 0) is received
- **THEN** a synth voice is triggered with the corresponding note and velocity on the currently selected synth preset

#### Scenario: Velocity sensitivity
- **WHEN** a MIDI note-on is received with velocity 64
- **THEN** the synth voice is triggered at approximately half amplitude (velocity 64/127 ≈ 0.5)

### Requirement: MIDI note-off releases synth voice
The system SHALL release synth voices when MIDI note-off messages are received.

#### Scenario: Release a note
- **WHEN** a MIDI note-off message (status 0x80, or 0x90 with velocity 0) is received
- **THEN** the matching synth voice enters the envelope release phase

#### Scenario: Multiple held notes
- **WHEN** multiple MIDI notes are held simultaneously and one is released
- **THEN** only the released note's voice enters release; other held notes continue sounding

### Requirement: MIDI input in standalone only
MIDI device input SHALL only be available in standalone frontends (ImGui and GTK), not in plugin builds.

#### Scenario: Plugin builds
- **WHEN** running as a VST3 or CLAP plugin
- **THEN** no MIDI device enumeration or selection is shown (the DAW provides MIDI input)

### Requirement: Frontend parity
Both ImGui and GTK standalone frontends SHALL implement identical MIDI device selection in the settings panel.

#### Scenario: ImGui MIDI settings
- **WHEN** using the ImGui standalone
- **THEN** the settings panel includes a MIDI input device dropdown with select functionality

#### Scenario: GTK MIDI settings
- **WHEN** using the GTK standalone
- **THEN** the settings panel includes a MIDI input device dropdown with identical functionality
