## ADDED Requirements

### Requirement: Drum grid step display
The drum grid SHALL display a grid of steps for each track, with track labels, volume/pan controls, mute/solo buttons, and a visual playhead.

#### Scenario: Grid renders with correct step count
- **WHEN** a pattern with 16 steps and 8 tracks is loaded
- **THEN** the grid shows 8 rows x 16 columns of clickable step cells

### Requirement: Step click and drag
The user SHALL be able to click individual steps to toggle them on/off and click-drag across multiple steps to set them all.

#### Scenario: Click-drag to fill steps
- **WHEN** the user clicks step 0 and drags to step 3
- **THEN** steps 0-3 are all toggled to the same state (on if first was off, off if first was on)

### Requirement: Right-click step editing
Right-clicking a step SHALL show a popup with velocity and note controls.

#### Scenario: Right-click popup
- **WHEN** the user right-clicks an active step
- **THEN** a popup appears with velocity slider (0-127) and note selector

### Requirement: Track type selection
Each track SHALL have a dropdown to select its type (sampler or synth) and the associated sample/preset.

#### Scenario: Change track to synth
- **WHEN** the user selects "Synth" type for a track
- **THEN** the track switches to synth mode and synth editor becomes available

### Requirement: Playhead visualization
The current playback step SHALL be highlighted in the grid during playback.

#### Scenario: Playhead advances
- **WHEN** transport is playing at 120 BPM
- **THEN** the highlighted column advances through the grid in time with the beat
