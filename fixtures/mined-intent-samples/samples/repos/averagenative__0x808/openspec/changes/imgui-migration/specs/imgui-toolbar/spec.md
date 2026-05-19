## ADDED Requirements

### Requirement: Toolbar always visible
The toolbar SHALL render as a fixed panel at the top of the window (y=0, full width, 60px height) and SHALL always be visible regardless of which content panel has focus.

#### Scenario: Toolbar visible on first frame
- **WHEN** the application starts and the first frame renders
- **THEN** the toolbar is visible with all buttons rendered

#### Scenario: Toolbar persists during panel changes
- **WHEN** the user toggles between PAT/SONG/PERF modes, opens the browser, or resizes the window
- **THEN** the toolbar remains visible and interactive throughout

### Requirement: Toolbar transport controls
The toolbar SHALL contain PLAY/STOP, REC, BPM slider (40-300), Swing slider (0-100%), and Volume slider (0-100%).

#### Scenario: Play/Stop toggle
- **WHEN** the user clicks the PLAY button
- **THEN** transport starts playing and button label changes to STOP

#### Scenario: BPM slider drag
- **WHEN** the user drags the BPM slider
- **THEN** engine BPM updates in real-time

### Requirement: Toolbar panel toggles
The toolbar SHALL contain buttons for EXPORT, PRESETS, PIANO, PAT/SONG/PERF, FX, BROWSE, and EXIT. Active toggles SHALL have highlighted colors.

#### Scenario: Toggle button activation
- **WHEN** the user clicks FX button
- **THEN** the mixer panel appears and the FX button shows highlighted color

### Requirement: Toolbar status messages
The toolbar SHALL display temporary status messages (save confirmations, pattern changes) that auto-dismiss after ~3 seconds.

#### Scenario: Status message display
- **WHEN** the user presses Ctrl+S and save succeeds
- **THEN** "Saved: project.sqproj" appears in the toolbar and fades after 3 seconds
