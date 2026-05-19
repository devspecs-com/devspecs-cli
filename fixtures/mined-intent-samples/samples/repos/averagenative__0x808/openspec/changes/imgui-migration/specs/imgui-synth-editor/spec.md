## ADDED Requirements

### Requirement: Synth parameter editing
The synth editor SHALL display and allow editing of all synth preset parameters: oscillator, filter, ADSR envelope, effects, and mode-specific parameters (subtractive/FM/wavetable).

#### Scenario: Edit filter cutoff
- **WHEN** the user drags the filter cutoff knob
- **THEN** the synth preset's cutoff value updates in real-time

### Requirement: Preset selector with scroll wheel
The synth editor SHALL have a preset dropdown that supports scroll wheel cycling when hovered.

#### Scenario: Scroll wheel preset cycling
- **WHEN** the user hovers over the preset dropdown and scrolls the mouse wheel up
- **THEN** the previous preset is selected (wrapping from first to last)

### Requirement: Mode selector with scroll wheel
The synth mode dropdown (Subtractive/FM/Wavetable) SHALL support scroll wheel cycling when hovered.

#### Scenario: Scroll wheel mode cycling
- **WHEN** the user hovers over the mode dropdown and scrolls down
- **THEN** the next mode is selected (wrapping from Wavetable back to Subtractive)

### Requirement: ADSR visualization
The synth editor SHALL display a visual representation of the ADSR envelope curve.

#### Scenario: ADSR curve display
- **WHEN** the synth editor is visible
- **THEN** an ADSR curve is drawn showing attack, decay, sustain level, and release phases

### Requirement: Custom knob widgets
Synth parameters SHALL use rotary knob widgets with click-drag interaction for precise value control.

#### Scenario: Knob drag interaction
- **WHEN** the user clicks and drags vertically on a knob
- **THEN** the parameter value changes proportionally to drag distance
