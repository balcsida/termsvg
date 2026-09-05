## 1. Record a baseline without compression

- [x] 1.1 Confirm repository instructions, active feature branch, and working-tree state. Preserve unrelated files and use an isolated checkout for checks that regenerate examples. Record the source SHA and current Taskfile example recipes.
- [x] 1.2 Before changing renderer code, run `go test ./pkg/renderer/svg ./pkg/ir ./internal/svgoutput ./cmd/termsvg/export` in the isolated checkout and record the actual result. Inspect status afterward for generated-fixture writes. Do not run compression tests or the `scripts/svgmetrics` package.
- [x] 1.3 Produce all six baseline SVGs from current source at temporary paths with their current recipes. Record final uncompressed bytes and `MeasureCandidate` structural metrics, validating reported bytes against the final output stream. Do not invoke the existing compression-enabled matrix/metrics path.
- [x] 1.4 Run existing targeted benchmarks with `go test ./pkg/renderer/svg -run '^$' -bench 'Benchmark(AutoSelection|SelectedSerialization|CandidateMatrix)' -benchtime=1x -benchmem -count=5`; record export timing and allocations separately from browser results.
- [x] 1.5 Establish the browser protocol from design.md using available browser tooling and the existing embedding fixture pattern. Record browser versions, fonts, viewport, embedding mode, measured interval, warmup, and paired repetitions. Capture loading, painting/playback, and memory evidence where available, marking missing targets or metrics unmeasured. Store a baseline report under this change directory.

## 2. Reuse recurring complete states

- [x] 2.1 Trace every `contentKeyframesFor` caller and extend existing tests with profitable A/B/A recurrence, empty states, hash collisions, and a nonprofitable fallback case. Run the focused cases before implementation and record the expected failure.
- [x] 2.2 Implement deterministic timeline-local state interning with full equality after hashing, preserving original selectors and immutable IR. Update dependent strip/href/track preparation and complete cost accounting; retain the original representation on equal or worse final cost.
- [x] 2.3 Verify with `go test ./pkg/renderer/svg` and existing semantic, cursor, finite-loop, wide-glyph, and cost-parity coverage. Compare recurrence-heavy fixtures and all six outputs against the baseline, including actual browser behavior. Record gains, neutral cases, and regressions independently.

## 3. Broaden opt-in automatic selection

- [x] 3.1 Add focused failing option/selection tests for SMIL switching auto, CSS rejection, explicit-setting preservation, eligible scroll selection, endpoint-based scroll exclusion, stable ties, exact costs, shared-plan immutability, and single winner serialization. Run the relevant cases before implementation.
- [x] 3.2 Extend the bounded candidate loop with eligible scroll candidates and `FrameSwitchAuto`. Preserve fixed animation/style/primitives/FPS, existing concrete modes, and old candidate tie preference. Add candidate switching identity to debug diagnostics without introducing a new optimizer framework.
- [x] 3.3 Update CLI validation/help and README for `--svg-frame-switch=auto`, including its SMIL requirement and the difference between size selection and structural runtime estimates. Leave defaults and retained-primitive validation unchanged.
- [x] 3.4 Run `go test ./pkg/renderer/svg ./cmd/termsvg/export` and repeat the targeted selection benchmarks. Compare all examples against old auto selection and explicit winning profiles with identical options. Record increased export cost alongside size and browser results.

## 4. Evaluate text-prefix reuse

- [x] 4.1 Measure a bounded same-position/same-style prefix candidate on 444816 and a progressive-text fixture, including complete definition/reference/style/suffix overhead. Document the proposed safe subset and a maximum of one prototype representation before writing production changes.
- [x] 4.2 Add a focused runnable check for glyph placement and fallback behavior, then prototype the candidate in isolation. Check font shaping, ligatures, fallback fonts, wide/combining glyphs, underline, and embedding interactions; unsupported or uncertain cases must use unsplit text.
- [x] 4.3 Repeat final-byte, structural, export, and browser measurements using the established protocol. If gains and fidelity are demonstrated, integrate only the safe subset with exact-cost fallback; otherwise remove the prototype. Record acceptance or rejection and evidence in the change report. A measured rejection completes this task.

## 5. Verify and summarize outcomes

- [x] 5.1 Run `go test ./pkg/renderer/svg ./pkg/ir ./internal/svgoutput ./cmd/termsvg/export`, `go vet ./pkg/renderer/svg ./pkg/ir ./internal/svgoutput ./cmd/termsvg/export`, and `git diff --check`. Use temporary build outputs and an isolated checkout for commands that regenerate fixtures; do not run compression tests.
- [x] 5.2 Re-run the six-example comparisons for the final combined changes. Verify determinism, exact output costs, unchanged timing/FPS, semantic and visual parity, and the browser acceptance gate. Do not treat source-node reductions or requestAnimationFrame samples alone as measured SVG paint improvements.
- [x] 5.3 Update example recipes/artifacts only where smaller output passes the browser gate; keep current profiles for regressions or unresolved tradeoffs. Preserve the canonical 256colors fixture expected by integration tests.
- [x] 5.4 Complete the report with per-example before/after uncompressed bytes, export cost, browser coverage/results, retained and rejected experiments, and any limitations. Verify no compression execution/results or unrelated file changes entered the work. Keep commits independently reversible and run `git status` before each commit.
