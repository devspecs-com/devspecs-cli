## ADDED Requirements

### Requirement: Piano roll note grid
The piano roll SHALL display a grid of MIDI notes (rows) by time steps (columns) with a keyboard reference on the left side.

#### Scenario: Note grid renders
- **WHEN** a synth track is selected
- **THEN** the piano roll shows a scrollable grid with note rows and step columns

### Requirement: Note placement and deletion
The user SHALL click to place notes and click existing notes to delete them.

#### Scenario: Place a note
- **WHEN** the user clicks on step 4, note C4
- **THEN** a note is placed at that position with default velocity 100

#### Scenario: Delete a note
- **WHEN** the user clicks on an existing note
- **THEN** the note is removed

### Requirement: Piano roll playhead
The piano roll SHALL show a vertical playhead line during playback.

#### Scenario: Playhead during playback
- **WHEN** transport is playing
- **THEN** a vertical line advances through the piano roll in sync with the beat
