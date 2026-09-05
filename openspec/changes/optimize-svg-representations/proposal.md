## Why

TermSVG already supports several compact SVG representations, but auto selection considers only three layouts with a fixed switching mechanism, and recurring content states are stored again after intervening changes. We should measure and try the remaining representation improvements while preserving visual behavior and distinguishing browser playback cost from structural estimates.

## What Changes

- Establish comparable baselines for all six example outputs and targeted recurrence, scrolling, and prefix-heavy fixtures. Measure final uncompressed SVG bytes, structure, export cost, and actual browser loading and playback.
- Extend opt-in auto layout selection to eligible scroll candidates and add explicit opt-in automatic frame switching for comparing translation with SMIL `href`. Preserve existing defaults and explicit choices.
- Reuse nonconsecutive equal content states within a timeline without changing transition times, cursor behavior, or terminal geometry.
- Conduct a bounded text-prefix reuse experiment. Retain production changes only if final-byte savings survive reference overhead, visual checks, and browser-performance checks; otherwise document the result and remove the prototype.
- Exclude compression entirely: no gzip, Brotli, SVGZ, compressed-size measurements, or hosting changes.

## Capabilities

### New Capabilities

- `svg-representation-selection`: Deterministic, opt-in selection among compatible SVG layout and switching candidates, with preserved explicit settings and measured tradeoffs.
- `svg-content-reuse`: Lossless reuse of recurring SVG content states and acceptance criteria for the optional text-prefix experiment.

### Modified Capabilities

None. This repository has no existing main OpenSpec capability specifications.

## Impact

- Renderer selection, content preparation, options, and cost/structural accounting in `pkg/renderer/svg/`.
- CLI validation and help in `cmd/termsvg/export/`, plus relevant README documentation.
- Existing SVG parity tests, benchmarks, browser fixtures, and example comparison tooling. Example recipes change only after evidence supports a better profile.
- No new production dependencies, embedded scripts, default FPS cap, font substitution, or rasterization. Current recording IR remains immutable.
