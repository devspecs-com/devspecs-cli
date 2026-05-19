## ADDED Requirements

### Requirement: Mixer view
The mixer view SHALL display per-track volume sliders, pan controls, mute/solo buttons, and effect slot assignments.

#### Scenario: Mixer displays all tracks
- **WHEN** the user enables FX view
- **THEN** the mixer shows vertical channel strips for each track with volume, pan, mute, solo, and effect dropdowns

### Requirement: Sample browser
The sample browser SHALL display available WAV files in a scrollable list, with preview playback and drag-to-assign functionality.

#### Scenario: Browse and assign sample
- **WHEN** the user opens the browser and clicks a sample filename
- **THEN** the sample is assigned to the currently selected sampler track

### Requirement: Virtual keyboard
The virtual keyboard SHALL render a 3-octave piano at the bottom of the screen with clickable keys and octave shift buttons.

#### Scenario: Click piano key
- **WHEN** the user clicks a key on the virtual keyboard
- **THEN** the corresponding note is triggered on the active synth preset

### Requirement: Arrangement view
The arrangement panel SHALL display pattern chain slots for SONG mode with pattern selection per slot.

#### Scenario: Arrangement in SONG mode
- **WHEN** the user switches to SONG mode
- **THEN** the arrangement panel appears showing a sequence of pattern slots

### Requirement: Pattern presets dialog
The pattern presets dialog SHALL allow saving and loading named pattern presets.

#### Scenario: Load preset pattern
- **WHEN** the user opens presets and selects "Classic 808"
- **THEN** the current pattern is replaced with the preset pattern

### Requirement: Export dialog
The export dialog SHALL allow configuring WAV export settings (filename, bit depth, length) and triggering offline render.

#### Scenario: Export WAV
- **WHEN** the user configures export settings and clicks Export
- **THEN** the engine renders offline and saves a WAV file
