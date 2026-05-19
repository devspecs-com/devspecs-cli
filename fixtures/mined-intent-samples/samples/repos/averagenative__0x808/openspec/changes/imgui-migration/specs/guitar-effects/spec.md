## ADDED Requirements

### Requirement: Overdrive effect
The engine SHALL provide an overdrive effect with drive (0-1), tone (0-1), and mix (0-1) parameters. It SHALL apply soft-clipping saturation to the signal.

#### Scenario: Overdrive on drum bus
- **WHEN** overdrive is assigned to a track with drive=0.7, tone=0.5, mix=1.0
- **THEN** the audio output exhibits warm harmonic saturation with soft clipping

#### Scenario: Overdrive bypass
- **WHEN** overdrive mix is set to 0.0
- **THEN** the audio output is unchanged (dry signal only)

### Requirement: Fuzz effect
The engine SHALL provide a fuzz effect with gain (0-1), tone (0-1), and mix (0-1) parameters. It SHALL apply hard-clipping distortion for aggressive tones.

#### Scenario: Fuzz on synth track
- **WHEN** fuzz is assigned to a synth track with gain=0.8, tone=0.6, mix=1.0
- **THEN** the audio output exhibits aggressive hard-clipped distortion

### Requirement: Chorus effect
The engine SHALL provide a chorus effect with rate (0.1-10 Hz), depth (0-1), and mix (0-1) parameters. It SHALL apply modulated delay to create a thickened sound.

#### Scenario: Chorus on synth pad
- **WHEN** chorus is assigned to a track with rate=1.5, depth=0.5, mix=0.5
- **THEN** the audio output has a shimmering, widened quality from modulated delay

#### Scenario: Chorus parameter range
- **WHEN** chorus rate is set to minimum (0.1 Hz)
- **THEN** the modulation is slow and subtle, creating a gentle movement
