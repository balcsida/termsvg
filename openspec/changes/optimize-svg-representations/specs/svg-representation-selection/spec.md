## ADDED Requirements

### Requirement: Explicitly bounded automatic selection
The renderer SHALL preserve existing default settings and concrete layout, animation, switching, style, primitive, and FPS choices. Layout auto SHALL compare frames, bands, regions, and semantically eligible scroll candidates. Switching auto SHALL require SMIL and compare translation with href only for layouts authorized by the layout setting.

#### Scenario: Concrete switching remains fixed
- **WHEN** a user selects layout auto and concrete href switching with SMIL
- **THEN** every evaluated candidate uses href switching and the supplied style, primitive mode, FPS, and animation settings

#### Scenario: Both selection dimensions are automatic
- **WHEN** a user selects layout auto and switching auto with SMIL
- **THEN** the renderer compares translation and href for each eligible layout using a shared immutable semantic plan

#### Scenario: Concrete layout remains fixed
- **WHEN** a user selects bands with switching auto and SMIL
- **THEN** only bands candidates are compared

#### Scenario: CSS rejects automatic switching
- **WHEN** switching auto is requested with CSS animation
- **THEN** validation reports that automatic switching requires SMIL

#### Scenario: Scroll changes endpoint behavior
- **WHEN** a scroll candidate cannot preserve the reference playback's endpoint or post-end state
- **THEN** automatic selection excludes that candidate without changing explicit layout behavior

### Requirement: Deterministic selection with accurate costs
Size selection SHALL choose the smallest final uncompressed serialization from eligible candidates, including the existing candidate set. Complete ties SHALL retain existing layout preference before newly added candidates and prefer translation over href when switching is automatic. Runtime selection SHALL remain a documented structural estimate. Candidate metrics SHALL correspond to actual serialized output and its instantiated structure.

#### Scenario: Expanded search offers no improvement
- **WHEN** newly eligible candidates are equal to or larger than the old winner under size selection
- **THEN** automatic selection retains the old winner

#### Scenario: Selection is repeatable
- **WHEN** the same recording and options are rendered repeatedly
- **THEN** the selected representation, IDs, and serialized bytes are deterministic and the recording is unchanged

#### Scenario: Debugging a selected representation
- **WHEN** debug output is enabled
- **THEN** it identifies each candidate's layout, switching mode, objective, measured cost, and the selected representation

### Requirement: Evidence for representation recommendations
Evaluation SHALL compare all six example outputs under matched timing, fonts, and rendering settings, including current recipes as baselines. Reports SHALL separate final uncompressed bytes, export cost, structural estimates, and actual browser measurements. Evaluation SHALL NOT run compression, report compressed-size metrics, or modify hosting. A recommended replacement example profile SHALL reduce final bytes without a reproducible material browser regression on tested targets; missing coverage SHALL be explicitly identified.

#### Scenario: A smaller candidate is slower in the browser
- **WHEN** a candidate saves bytes but exhibits a reproducible material loading, playback, or memory regression
- **THEN** the report records the tradeoff and retains the current example profile

#### Scenario: A browser or metric is unavailable
- **WHEN** a target browser or meaningful memory metric cannot be measured
- **THEN** the report marks it unmeasured and makes no improvement claim for that coverage

#### Scenario: Existing metrics tooling invokes compression
- **WHEN** choosing a measurement path
- **THEN** evaluation uses an uncompressed-only path instead of executing the compression operation
