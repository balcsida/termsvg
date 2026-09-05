# SVG representation experiments

Source baseline: `bea65ba5c8a91aadc42bb5877efcab60e6f46897`, the local head of the existing SVG optimization branch. Implementation is isolated on `perf/svg-representations` at `/private/tmp/termsvg-representations`. No remote changes were made.

## Implemented

- `b21fb5c`: canonical minified paint names are shared by emission and cost accounting, using the installed minifier's color table. This fixes pre-existing measured-versus-final discrepancies of 3 bytes for 256colors and 11 for htop.
- `4048b6b`: automatic layout selection includes eligible scroll candidates; explicit `--svg-frame-switch=auto` compares translation with href under SMIL. Concrete settings and defaults remain fixed. Runtime selection remains a structural estimate.
- `f706c8e`: recurring complete states share timeline-local slots, preserving original selectors, cursor behavior, and immutable input. Full equality resolves hash collisions. Complete content costs select reuse only on a strict byte improvement.

All production commits have verified SSH signatures. The reviewed test-oracle correction is committed separately as `286dbf0`. Prefix sharing was rejected; see [prefix-report.md](prefix-report.md).

## Final uncompressed bytes

These comparisons preserve each original recipe's minification, font, window, SMIL, style, and FPS settings. The 256colors recipe remains uncapped; the other five retain their existing 30-FPS setting. No compression was executed or measured.

| Output | Baseline | Same explicit profile after changes | Smallest explored auto result | Selected for browser timing |
|---|---:|---:|---:|---:|
| 256colors | 63,564 | 63,533 | 63,533 | 63,533 |
| 444816 | 259,469 | 259,452 | 242,264 | 259,452 |
| 444816 borderless | 259,324 | 259,307 | 242,119 | 259,307 |
| htop | 25,608 | 25,549 | 25,549 | 25,549 |
| rgb | 6,749 | 6,748 | 6,335 | 6,335 |
| session | 34,786 | 34,633 | 34,633 | 34,633 |

The largest visually equivalent explored size reduction is rgb's 414 bytes (6.13%), using regions/href rather than regions/translation. This uses an existing renderer representation exposed through the broader selector; it is not a new drawing backend. Chrome runtime measurements reject that profile: total Paint work rises from 2.321 to 20.432 ms in standalone embedding and from 5.841 to 45.977 ms in image embedding. Every one of five paired runs per mode shows the large regression. The original regions/translation recipe is retained, yielding 6,748 bytes after the code changes. Recurrence and paint-cost improvements under unchanged explicit profiles are much smaller on these already optimized examples.

Both 444816 auto results are about 6.6% smaller, but regions versus the original frames layout changes glyph-edge rasterization. They are not selected for recipe promotion. Old-regions versus new-regions controls are pixel-identical, so the difference is a pre-existing layout tradeoff, not recurrence corruption.

The implemented same-profile savings range from 1 byte (rgb) to 153 bytes (session), at most about 0.44%. Broader auto selection remains an explicit size/runtime-estimate option; browser acceptance is a separate decision.

## Visual checks

All six selected artifacts pass exact-time comparisons at start, representative transitions and duration/end boundaries. Both final frames-based 444 outputs independently pass 42/42 comparisons across standalone, object, and inline embedding. The retained rgb regions/translation output separately passes all 24 exact-time comparisons.

The initial browser harness paused only the root SVG, leaving nested SVG time containers running. That falsely suggested an initial-state leak. The corrected harness pauses and seeks every SVG element, records nested clocks, and verifies baseline-versus-baseline controls. Corrected frames-versus-regions 444 comparisons still differ in 30/42 samples; matched old-regions/new-regions comparisons pass 42/42.

## Renderer/export cost

The first recurrence implementation repeated region optimization and was rejected: one uncapped 444816 region case increased from 7.53 s to 17.72 s, with allocations rising from 10.71 GB to 23.24 GB. The final implementation optimizes a layout once, then rematerializes only its chosen state partition. It deliberately does not search for a new region partition after interning.

Matched 30-FPS region checks after that correction show 0.2–0.4% allocation overhead. Remaining preparation overhead is visible in small frame/band cases. The following existing `BenchmarkCandidateMatrix` cases measure rendering prepared recording fixtures with legacy style, not full CLI processing or browser playback. Each median uses five one-iteration samples; timings on the shared machine are indicative, while allocation counts provide a less noisy comparison.

| Matched renderer case | Baseline median | Final median | Baseline allocated bytes | Final allocated bytes |
|---|---:|---:|---:|---:|
| 444816 / 30 FPS / frames-SMIL-href | 5.454 ms | 9.328 ms | 11,094,752 | 12,838,528 |
| borderless / 30 FPS / frames-SMIL-href | 4.945 ms | 9.345 ms | 11,094,512 | 12,833,176 |
| 256colors / uncapped / bands-SMIL-href | 2.051 ms | 2.644 ms | 4,337,608 | 4,813,088 |
| htop / 30 FPS / frames-SMIL-href | 0.480 ms | 0.804 ms | 1,094,920 | 1,282,616 |
| rgb / 30 FPS / regions-SMIL-translate | 7.551 ms | 7.704 ms | 9,712,920 | 9,720,232 |
| session / 30 FPS / bands-SMIL-href | 1.093 ms | 1.543 ms | 2,830,024 | 3,135,416 |

Selected serialization stays approximately 0.007 ms and 66,520 allocated bytes on the existing synthetic benchmark. Automatic selection still costs more than directly requesting a known winning profile; examples should use concrete profiles after measurement rather than repeat that search on every export.

## Browser runtime and profile acceptance

The main paired series is complete: 144 Chrome runs and 144 Safari runs, followed by a targeted dense-session comparison in both browsers. The timed series uses isolated browser sessions, no concurrent Go workloads, alternating baseline/candidate order, warmup plus five measured runs, and dedicated playback windows. See [browser-report.md](browser-report.md) for versions, launch conditions, trace evidence, metric limitations, and paired results. Source-node reductions alone are not treated as browser-speed improvements.

Chrome demonstrates why that distinction matters: rgb's href candidate has fewer source nodes (132 to 119), but estimated instantiated nodes increase and actual Paint work is roughly eight times higher. Animation-frame p95 remains around 17–18 ms, so frame-callback timing alone would have missed this additional work. The dense-session follow-up covers 26.75–29.75 seconds, including 36 state changes at 14 timestamps. Chrome Paint medians are 82.014 versus 81.801 ms; the median paired change is −0.8%, with a bootstrap 95% interval of −1.7% to +2.4%. Safari frame-interval p95 remains 19 ms; its Paint and memory costs are unmeasured. This supports no demonstrated dense-session paint regression, not a general browser-speed claim.

Existing example recipes and tracked SVG artifacts remain unchanged. The htop image load result also regresses by a median paired 22.6% (95% interval +9.9% to +25.5%), roughly 10 to 12 ms, while its Paint interval crosses zero. Other loading/paint intervals are broad, and the same-profile byte gains are small; generated candidates are evaluation evidence only. The larger auto-profile size wins fail the fidelity or runtime gate. No artifact promotion is warranted.

## Verification and reproducibility

- `go test ./pkg/renderer/svg ./pkg/ir ./internal/svgoutput ./cmd/termsvg/export` passes; final check including the test correction: SVG 12.135 s, export and IR cached, SVG output package has no tests. The focused recurrence cost check also passes uncached in 0.257 s.
- `go vet` on those packages passes.
- New-code lint reports zero issues. Full scoped lint on baseline and final source reports the same 90 issues, matched by linter, text, filename, and source line; unrelated baseline lint debt remains unchanged.
- Paint and selection task reviews approve without findings. Recurrence and final reviews approve production. The minor test-model correction aligning the expected candidate with fixed-partition reuse is addressed, re-reviewed, and verified.
- Actual final lengths agree with corrected candidate costs for all six explored auto outputs. Baseline reports retain actual file lengths rather than the pre-existing inaccurate paint estimates.

Reproduction artifacts are under `/private/tmp/termsvg-measurements/`: exact recipe JSON, binary/output hashes, raw browser runs, traces, screenshots, metrics and renderer benchmark summaries. Baseline and final benchmark logs are `/private/tmp/termsvg-baseline-bench.txt` and `/private/tmp/termsvg-final-bench.txt`. The interrupted whole-matrix candidate run documents a rejected intermediate implementation and is not final performance evidence. No compression-enabled matrix command or compression test package was run.
