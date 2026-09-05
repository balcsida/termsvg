## ADDED Requirements

### Requirement: Recurring content shares a state representation
Content preparation SHALL consider reusing nonconsecutive semantically equal complete states within each timeline, preserving every transition selector and duration. Equality SHALL include ordered rows, local positions, text, visual extents, and paint attributes. Hash collisions SHALL be resolved by full semantic equality. Reuse SHALL be selected only when the complete final-byte cost improves over the original representation.

#### Scenario: A state recurs after another state
- **WHEN** a timeline contains A, B, A and state reuse is profitable
- **THEN** its representation stores A and B once and maps the original selectors to state indices 0, 1, 0

#### Scenario: Distinct states share a hash
- **WHEN** two semantically different states have the same hash
- **THEN** they remain distinct states

#### Scenario: Empty state and cursor-only transitions
- **WHEN** content returns to an empty state or the cursor changes while content remains equal
- **THEN** empty content is represented correctly and the independent cursor timeline is preserved

#### Scenario: Changed row-definition profitability outweighs reuse
- **WHEN** state reuse changes reference counts such that the complete result is not smaller
- **THEN** the renderer retains the original representation

### Requirement: Reuse preserves all supported playback behavior
Reused states SHALL preserve visible cells, colored blanks, underlined whitespace, wide and combining glyphs, cursor behavior, transitions, loop boundaries, and finite playback endpoints for every admitted representation. Candidate preparation SHALL NOT mutate input recording or shared semantic state. Costs and structural metrics SHALL reflect the selected state mapping.

#### Scenario: Both switching mechanisms use the reused timeline
- **WHEN** the same recurring timeline is rendered with translation and SMIL href switching
- **THEN** both display the original state at each transition and boundary despite reusing state indices

#### Scenario: Local viewport states look alike at different coordinates
- **WHEN** states in distinct local timelines contain similar text
- **THEN** timeline-local reuse preserves their positions and does not incorrectly merge coordinate contexts

### Requirement: Text-prefix reuse is conditional on demonstrated benefit
The prefix experiment SHALL measure complete serialization and browser cost against unsplit text and SHALL preserve rendered glyph placement and styling. Unsupported or uncertain text-shaping cases SHALL retain the original text representation. Production prefix reuse SHALL be retained only when it passes semantic and visual checks, yields final-byte savings, and avoids a reproducible material browser regression on tested targets. A rejected experiment SHALL leave no dormant production implementation or speculative configuration.

#### Scenario: Safe shared prefix yields a net improvement
- **WHEN** a bounded prefix representation passes visual parity and improves final-byte cost without measured browser regression
- **THEN** only that demonstrated safe subset is eligible for reuse with an unsplit fallback

#### Scenario: A split can change glyph shaping or decoration
- **WHEN** prefix splitting has uncertain effects on ligatures, font fallback, glyph positioning, combining characters, or underlining
- **THEN** the renderer retains the unsplit text node

#### Scenario: References erase the apparent savings
- **WHEN** prefix definitions, references, styles, or additional runtime nodes eliminate the benefit
- **THEN** the experiment is recorded as rejected and prototype production changes are removed
