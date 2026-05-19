## ADDED Requirements

### Requirement: Sequencer sends note-off based on step length
The sequencer SHALL release synth voices when a note's step length expires, transitioning the voice's amplitude envelope to the release phase.

#### Scenario: Short note with release tail
- **WHEN** a synth step has velocity > 0 and length = 2.0 (2 steps)
- **THEN** the synth voice is triggered on the step boundary and enters envelope release after 2 steps have elapsed

#### Scenario: Zero-length note uses one-shot behavior
- **WHEN** a synth step has velocity > 0 and length = 0
- **THEN** the synth voice is triggered and no note-off is sent (current behavior — voice decays via ADSR with sustain=0)

#### Scenario: Note retrigger before length expires
- **WHEN** the same track triggers a new note before the previous note's length expires
- **THEN** the previous voice is released and a new voice is triggered

### Requirement: Sampler tracks unaffected
The note-off system SHALL only apply to synth and SF2 tracks. Sampler tracks continue to play samples to completion as before.

#### Scenario: Sampler track with length > 0
- **WHEN** a sampler track step has length > 0
- **THEN** the sample plays to completion (no early cutoff)

### Requirement: Active note tracking
The engine SHALL track which synth voices were triggered by the sequencer and their remaining duration, using a fixed-size array that requires no heap allocation.

#### Scenario: Maximum concurrent tracked notes
- **WHEN** 16 synth voices are active with pending note-offs
- **THEN** all are tracked and released at the correct times without memory allocation

#### Scenario: Transport stop releases all
- **WHEN** playback is stopped while notes are active
- **THEN** all tracked voices enter the release phase immediately
