# Browser representation measurements

Decision: retain all existing example recipes and tracked SVG artifacts. The generated candidates are evaluation evidence only. The smaller rgb href profile has a decisive traced Paint regression; the two smaller 444816 region profiles change glyph rasterization. Remaining small byte savings do not justify artifact churn while browser non-regression remains uncertain.

## Baseline and tooling

The six baseline artifacts were generated with `/private/tmp/termsvg-baseline` using the current Taskfile recipes, replacing only the output paths. Preserved artifacts: `/private/tmp/termsvg-measurements/baseline`; exact argv and final byte counts: `/private/tmp/termsvg-measurements/baseline-recipes.json`.

| Example | Baseline uncompressed bytes |
| --- | ---: |
| 256colors | 63,564 |
| 444816 | 259,469 |
| 444816_borderless | 259,324 |
| htop | 25,608 |
| rgb | 6,749 |
| session | 34,786 |

Tool discovery found the CUA MCP, which exposed native browser apps but no browser automation surfaces. No dedicated Playwright/CDP MCP was exposed. Existing Playwright 1.62.1 from the Codex runtime controls installed headed Google Chrome 152.0.7977.83. `/usr/bin/safaridriver` successfully created a Safari 26.6.2 session without changing system settings. The host is macOS 26.6.2 (25G83), Node v26.8.1. No package installation was needed.

## Protocol

`/private/tmp/termsvg-measurements/measure.cjs` serves files over localhost without caching. It visits each of the six examples in standalone SVG and HTML `img` modes. Chrome uses a 1280 x 900 CSS-pixel viewport and device scale 1. Safari final window size is 1280 x 990; actual inner viewport and devicePixelRatio are recorded per run. SVG font declaration is `Monaco,Consolas,'Courier New',monospace`; no font overrides are introduced. Playwright default launch flags disable background timer/renderer throttling and occluded-window backgrounding; `chrome-launch.txt` records the launch. Per-run visibility remains recorded. Computed family is recorded where SVG DOM access exists; actual resolved font is not independently verified.

Each example/mode has one excluded warmup and five measured runs. Paired comparisons alternate baseline/candidate order by repetition. Every run measures document loading and then a 3-second playback window. Navigation Timing load and DOM-ready measurements are retained separately from wall-clock driver navigation. Paint Timing entries are retained where exposed; first-contentful paint is a browser paint milestone, not proof of the first nonempty terminal frame. Chrome DevTools traces provide actual Paint event counts, durations, and paint-event intervals; representative raw traces and screenshots are retained for run 0. Chrome native process RSS and CDP heap/DOM metrics are recorded in the final protocol, with RSS summed over process IDs returned by that isolated browser instance. RSS includes browser overhead and is a snapshot, not peak SVG-only memory.

requestAnimationFrame p95 intervals and estimated missed 60-Hz opportunities (`sum(max(0, round(interval / (1000/60)) - 1))`) are scheduling diagnostics. They do not measure SVG paint. Multiple Paint events can belong to one frame; event interval/duration and counts must not be called presented-frame rate. Safari actual paint traces and native memory remain unmeasured unless a supported source becomes available. No performance claim substitutes rAF for SVG paint evidence.

`fidelity.cjs` compares paused SMIL renderings in standalone, object, and inline embedding modes at time zero, selected timeline key-time boundaries ±1 ms, and duration/end ±1 ms. Chrome canvas compares decoded screenshot pixels at identical times and viewport. These are neutral host wrappers; hostile host CSS is not included. This sampling is not exhaustive coverage of every frame or font configuration.

## Contention and acceptance

The initial `baseline-chrome` series overlaps Go benchmarks and is tooling-validation evidence only. It must not establish final performance acceptance. A contemporaneous process snapshot also showed substantial unrelated background CPU use. Final alternating pairs must run after the Go benchmarks stop; system contention and run variability remain limitations of this local machine.

Final comparisons and acceptance decisions are recorded below. No compression commands or measurements have been executed by this browser measurement work.

## Prefix experiment: rejected

The temporary 444816 prototype reduced final bytes from 259,469 to 258,558 (911 bytes, 0.351%). It failed pixel fidelity: 24 of 42 identical-time screenshot comparisons differed by 51 pixels, across standalone SVG, object, and inline modes. The corresponding baseline-versus-baseline control passed all 42 comparisons with zero differing pixels. Differences occurred in mid-playback and near the duration boundary, not only during document loading.

The concrete cause is the prototype's integer character-cell offset. Two shared prefixes have actual browser text advances of 840.140625 and 696.125 CSS pixels, while the suffix begins after 840 and 696 pixels respectively. The resulting -0.140625/-0.125-pixel shift changes glyph rasterization. Geometry probes with the normal font stack, Courier New override, and generic monospace retained fractional advances. The geometry probe used headless Chrome; pixel comparisons used the headed browser described above.

Evidence lives under `/private/tmp/termsvg-measurements`: `prefix-fidelity/results.json` and all screenshot pairs; `prefix-control/results.json`; `prefix-geometry.json`; and reproducible scripts `fidelity.cjs`/`prefix-geometry.cjs`. No prototype runtime measurement was needed after this demonstrated semantic rendering failure. This experiment must not enter production.

## Auto-profile fidelity screening

The first combined candidate replaced layout/frame-switch with `auto/auto` while retaining every recipe's minification, window, SMIL, font/style, and FPS settings. Its outputs and hashes are preserved in `candidate-auto/`, `candidate-auto-recipes.json`, and `candidate-auto-binary-sha256.txt` under the measurement directory.

| Example | Baseline bytes | Auto candidate bytes | Corrected exact-time pixel comparisons |
| --- | ---: | ---: | --- |
| 256colors | 63,564 | 63,533 | 42/42 identical |
| 444816 | 259,469 | 242,264 | 12/42 identical; retain frames recipe |
| 444816_borderless | 259,324 | 242,119 | 12/42 identical; retain frames recipe |
| htop | 25,608 | 25,549 | 42/42 identical |
| rgb | 6,749 | 6,335 | 24/24 identical |
| session | 34,786 | 34,633 | 42/42 identical |

The initial fidelity harness sought only the outer SVG time container. Nested SVG viewports have separate SMIL clocks, making those initial comparisons invalid for nested layouts. The corrected harness pauses and seeks **every** SVGSVGElement, then records each clock and paused state. All recorded clocks are paused with zero intra-document clock skew. Only `corrected-fidelity-*` results count for this table; the earlier `final-fidelity-*` and `bands-control` results are diagnostic artifacts, not acceptance evidence.

The smaller 444816 candidates use regions/href. They match terminal content and timing in visual inspection, but differ from the tuned frames profiles in glyph rasterization (for example, 2,216 pixels at 0.1 seconds with maximum channel difference 60). A matched old-regions/href versus new-regions/href control passes 42/42 screenshots exactly (`regions-control/results.json`), isolating this difference to the existing profile behavior rather than the new recurrence implementation. The strict current-profile visual gate therefore rejects these two auto profile promotions. Final browser timings used the existing frames options for these examples and the fidelity-passing candidates for the other four.

## Final selected timing inputs and window coverage

The final measurement binary is `/private/tmp/termsvg-final`, built from committed source `f706c8e`. Its SHA-256 and exact arguments are retained in `candidate-binary-sha256.txt` and `candidate-recipes.json`. The timed inputs are 63,533 / 259,452 / 259,307 / 25,549 / 6,335 / 34,633 bytes in the table's example order. Both 444816 variants retain frames/href and pass a fresh 42/42 exact-time pixel comparisons (`selected-fidelity-*`). The other four files are byte-identical to the earlier corrected-fidelity candidates.

The main series samples three seconds after document load. Chrome's main series did not retain explicit JavaScript window start/end timestamps; its raw traces retain navigation and paint timestamps. Safari retains explicit performance-clock bounds and SVG clocks where DOM access is available. This limitation does not turn the nominal three seconds into an exact timestamp measurement.

`transition-density.json` records serialized state changes in the first three animation seconds. It includes 24 of rgb's 28 changes at 12 distinct times; 89 of 444816's 372 changes; 14 of htop's 27 changes; 13 of 256colors' 120 changes; and only two of session's 123 changes. The session main window is sparse. A separate session dense-window series sought all SVG time containers to 26.75 seconds and played to approximately 29.75 seconds; this is a maximum-count three-second interval with 36 state changes at 14 distinct times. Actual run clocks are recorded.

### rgb profile: rejected on actual paint work

Chrome's five paired runs in each embedding mode show a consistent large increase in actual Paint work for the smaller regions/href candidate. Standalone median Paint duration summed across the trace rises from 2.321 to 20.432 ms, with Paint event counts rising from 23–25 to 361–363. In `img`, median Paint duration rises from 5.841 to 45.977 ms and event counts from 31–35 to 335–340. Every pair regresses strongly. These are traced Paint events, not an inference from source nodes or rAF. rAF p95 remains around 17–18 ms and would have missed this extra work.

The 6,335-byte href profile is rejected despite its 6.1% byte saving. Retain the original regions/translate recipe. No repeated runtime experiment is needed to establish this direction. The rejected timed artifact remains preserved; the regenerated final translate artifact is kept separately as `retained/rgb.svg` (6,748 bytes) and passed all 24 corrected exact-time fidelity comparisons before dense timing began. It is not adopted as a tracked artifact.

## Main-series medians

Each cell is baseline → generated candidate. Each median excludes the warmup and contains five runs. Chrome has 144 total runs including warmups; Safari also has 144. All runs report visible documents. Actual Safari viewport is 1280 × 900, device scale 1. Source `f706c8e` identifies the tested production code; the later test-only correction does not change these measured binaries.

### Chrome loading and native process RSS

| Example | Mode | Load ms | FCP ms | Process RSS MiB |
| --- | --- | ---: | ---: | ---: |
| 256colors | img | 12.7 → 12.1 | 52.0 → 48.0 | 1353.2 → 1351.2 |
| 256colors | standalone | 13.1 → 13.4 | 44.0 → 52.0 | 1137.2 → 1134.9 |
| 444816 | img | 88.1 → 92.1 | 128.0 → 120.0 | 1545.5 → 1544.4 |
| 444816 | standalone | 60.1 → 59.9 | 104.0 → 100.0 | 1309.4 → 1308.2 |
| 444816_borderless | img | 94.3 → 88.4 | 124.0 → 128.0 | 1803.5 → 1800.3 |
| 444816_borderless | standalone | 59.0 → 63.0 | 96.0 → 108.0 | 1326.2 → 1326.8 |
| htop | img | 10.1 → 12.1 | 48.0 → 48.0 | 1903.0 → 1902.6 |
| htop | standalone | 10.9 → 10.9 | 48.0 → 48.0 | 1333.1 → 1332.8 |
| rgb | img | 6.6 → 6.9 | 40.0 → 36.0 | 1803.5 → 1803.4 |
| rgb | standalone | 6.2 → 6.2 | 44.0 → 36.0 | 1338.1 → 1334.6 |
| session | img | 10.0 → 10.2 | 44.0 → 44.0 | 1806.0 → 1804.9 |
| session | standalone | 9.7 → 9.4 | 48.0 → 44.0 | 1342.4 → 1341.8 |

### Chrome traced Paint and scheduling

Main Paint totals cover navigation plus playback, rather than an exactly delimited three-second segment. Paint-event intervals reflect repaint activity, not presented-frame cadence: fewer paints can be a benefit, as the translate renderer demonstrates. The main trace may include a small amount of outgoing-document activity around navigation. Dense-session Paint measurements below are filtered to the explicitly recorded window.

| Example | Mode | Total Paint ms | p95 Paint duration ms | p95 Paint-event interval ms | p95 rAF interval ms | Estimated missed 60-Hz opportunities |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 256colors | img | 41.468 → 42.129 | 0.160 → 0.163 | 17.7 → 17.7 | 17.7 → 17.6 | 0 → 0 |
| 256colors | standalone | 13.758 → 14.350 | 0.075 → 0.070 | 18.1 → 18.1 | 18.3 → 18.2 | 0 → 0 |
| 444816 | img | 69.786 → 74.912 | 0.145 → 0.150 | 18.4 → 18.4 | 17.6 → 17.6 | 0 → 0 |
| 444816 | standalone | 15.501 → 15.772 | 0.078 → 0.070 | 19.1 → 18.9 | 18.4 → 18.5 | 0 → 0 |
| 444816_borderless | img | 67.667 → 70.458 | 0.144 → 0.142 | 18.2 → 18.3 | 17.2 → 17.5 | 0 → 0 |
| 444816_borderless | standalone | 15.948 → 16.003 | 0.074 → 0.080 | 17.8 → 19.5 | 18.0 → 18.0 | 0 → 0 |
| htop | img | 40.567 → 51.920 | 0.374 → 0.518 | 17.6 → 17.7 | 17.6 → 17.6 | 0 → 0 |
| htop | standalone | 24.075 → 24.315 | 0.265 → 0.267 | 17.9 → 17.8 | 18.0 → 18.1 | 0 → 0 |
| rgb | img | 5.841 → 45.977 | 0.373 → 0.276 | 299.9 → 17.4 | 17.4 → 17.4 | 0 → 0 |
| rgb | standalone | 2.321 → 20.432 | 0.151 → 0.167 | 366.7 → 17.5 | 18.1 → 18.0 | 0 → 0 |
| session | img | 37.908 → 39.281 | 0.150 → 0.152 | 17.6 → 17.6 | 17.5 → 17.4 | 0 → 0 |
| session | standalone | 11.704 → 13.525 | 0.062 → 0.063 | 17.1 → 17.2 | 17.5 → 17.4 | 0 → 0 |

### Safari loading and scheduling

Actual Paint traces, presented-frame misses, and meaningful memory measurements are unavailable through the Safari WebDriver surface used here. FCP and rAF do not fill those gaps.

| Example | Mode | Load ms | FCP ms | p95 rAF interval ms | Estimated missed 60-Hz opportunities |
| --- | --- | ---: | ---: | ---: | ---: |
| 256colors | img | 53.0 → 55.0 | 57.0 → 59.0 | 18.0 → 18.0 | 2 → 2 |
| 256colors | standalone | 51.0 → 59.0 | 56.0 → 63.0 | 18.0 → 18.0 | 1 → 0 |
| 444816 | img | 118.0 → 113.0 | 122.0 → 124.0 | 18.0 → 18.0 | 4 → 2 |
| 444816 | standalone | 102.0 → 105.0 | 142.0 → 146.0 | 18.0 → 18.0 | 1 → 0 |
| 444816_borderless | img | 115.0 → 118.0 | 124.0 → 121.0 | 18.0 → 18.0 | 2 → 2 |
| 444816_borderless | standalone | 110.0 → 110.0 | 152.0 → 152.0 | 18.0 → 18.0 | 2 → 1 |
| htop | img | 48.0 → 51.0 | 53.0 → 55.0 | 18.0 → 18.0 | 2 → 1 |
| htop | standalone | 58.0 → 59.0 | 61.0 → 62.0 | 18.0 → 18.0 | 2 → 1 |
| rgb | img | 45.0 → 43.0 | 50.0 → 48.0 | 18.0 → 18.0 | 3 → 2 |
| rgb | standalone | 41.0 → 43.0 | 42.0 → 46.0 | 18.0 → 18.0 | 2 → 0 |
| session | img | 44.0 → 43.0 | 46.0 → 49.0 | 18.0 → 18.0 | 2 → 1 |
| session | standalone | 59.0 → 59.0 | 62.0 → 62.0 | 18.0 → 18.0 | 2 → 0 |

## Paired estimates and uncertainty

`summarize.py` calculates per-repetition candidate/baseline percentage changes, their median, and a descriptive 95% percentile bootstrap interval using 10,000 paired resamples with seed 20260905. These intervals are for the median **paired change**, so they need not equal the ratio of the separately reported medians. Five pairs on a shared, thermally uncontrolled workstation do not establish an across-platform performance guarantee. Several intervals include changes beyond the 5% materiality threshold. Background system/security/UI activity remained; there were no concurrent Go workloads during final timing. RSS is an isolated browser-process snapshot with accumulated caches, not SVG-only or peak memory.

| Example | Mode | Chrome load paired change [95% interval] | Chrome Paint paired change [95% interval] |
| --- | --- | ---: | ---: |
| 256colors | img | -6.7% [-14.9%, +29.4%] | +3.0% [-11.3%, +14.4%] |
| 256colors | standalone | +3.8% [-4.6%, +6.2%] | +0.9% [-11.5%, +6.9%] |
| 444816 | img | +6.1% [-6.3%, +88.8%] | +7.3% [-0.9%, +20.4%] |
| 444816 | standalone | +1.2% [-5.9%, +7.7%] | -0.3% [-13.6%, +30.1%] |
| 444816_borderless | img | -5.3% [-46.0%, +6.7%] | -4.5% [-10.4%, +6.1%] |
| 444816_borderless | standalone | +7.3% [-5.9%, +30.4%] | -2.1% [-23.6%, +12.4%] |
| htop | img | +22.6% [+9.9%, +25.5%] | +10.8% [-5.9%, +33.0%] |
| htop | standalone | -2.7% [-3.7%, -0.0%] | -3.3% [-13.0%, +2.2%] |
| rgb | img | -0.0% [-13.6%, +13.1%] | +668.5% [+493.7%, +850.6%] |
| rgb | standalone | +1.6% [-3.2%, +12.5%] | +697.5% [+620.8%, +953.1%] |
| session | img | -3.8% [-13.8%, +16.7%] | +2.7% [-6.4%, +25.7%] |
| session | standalone | -3.1% [-8.2%, -0.9%] | -2.0% [-6.8%, +42.1%] |

Full pairs and intervals for every measured metric, including Safari and RSS, are retained in `paired-summary.json`. rgb has a large positive Paint interval in both modes; rAF stability does not rescue that profile. Other observations are mixed: for example htop img load increases by a median paired 22.6% [9.9%, 25.5%], while its Paint interval crosses zero. No small generated artifact is promoted on those data.

## Session dense-window result

The additional standalone series contains one warmup and five alternating pairs per engine. Every SVG time container was paused, sought to 26.75 seconds, then unpaused. Recorded SVG clocks begin at 26.75 and finish near 29.76 seconds; actual per-run bounds are in `dense-{chrome,safari}/results.json`. All container clocks within a document agree in these recordings. This adds meaningful transition coverage; dense img playback remains unmeasured because its SVG clock is not exposed to the host document.

| Engine | Window-filtered total Paint ms | p95 rAF interval ms | Estimated missed 60-Hz opportunities | Native process RSS MiB |
| --- | ---: | ---: | ---: | ---: |
| chrome | 82.014 → 81.801 | 9.9 → 9.9 | 0 → 0 | 1211.2 → 1210.4 |
| safari | unmeasured | 19.0 → 19.0 | 1 → 1 | unmeasured |

Chrome's window-filtered Paint paired change is -0.8% [-1.7%, +2.4%]; no material dense-session Paint regression is demonstrated. Safari provides scheduling evidence only. Chrome's dense rAF cadence differs from its main-series cadence, so cross-series absolute frame/paint totals must not be compared; baseline/candidate pairs within each series remain matched. Actual display refresh and presented-frame misses are unmeasured. Dense results and all metric intervals are in `dense-summary.json`. The conservative decision remains to keep existing recipes and tracked artifacts.

## Reproduction and evidence

All measurement assets are under `/private/tmp/termsvg-measurements`. Commands used the already installed runtime, installed browsers, and standard libraries. No compression execution or measurements occurred.

```sh
node /private/tmp/termsvg-measurements/measure.cjs paired chrome
node /private/tmp/termsvg-measurements/measure.cjs paired safari
FILES=session MODES=standalone SEEK_SECONDS=26.75 OUTPUT_DIR=dense-chrome node /private/tmp/termsvg-measurements/measure.cjs paired chrome
FILES=session MODES=standalone SEEK_SECONDS=26.75 OUTPUT_DIR=dense-safari node /private/tmp/termsvg-measurements/measure.cjs paired safari
python3 /private/tmp/termsvg-measurements/summarize.py
SERIES=dense python3 /private/tmp/termsvg-measurements/summarize.py
```

The Safari driver was started with `/usr/bin/safaridriver -p 9515`; a standard `POST /session` with browserName safari created the session. `measure.cjs` contains that session ID, which must be replaced on a fresh driver session. Its current version adds explicit window-clock recording to the original Chrome main protocol; the recorded Chrome main limitation above remains. The localhost server binds 127.0.0.1:8765, uses no-store responses, and serves standalone/img wrappers. `fidelity.cjs <baseline.svg> <candidate.svg> <result-name>` binds port8766 and captures corrected standalone/object/inline comparisons.

Exact export argv, bytes, and hashes: `baseline-recipes.json`, `candidate-recipes.json`, `baseline-sha256.txt`, `candidate-binary-sha256.txt`, and the preserved `candidate-auto-*` records. Runtime evidence: `paired-{chrome,safari}/results.json`, representative run0 PNGs and Chrome traces, `dense-{chrome,safari}/results.json`, logs, `chrome-launch.txt`, and process snapshots. Transition coverage: `transition-density.json` and `session-dense-window.json`. Pixel evidence: `corrected-fidelity-*`, `selected-fidelity-*`, `regions-control`, `prefix-fidelity`, `prefix-control`, and `retained-fidelity-rgb`. Temporary evidence is not a replacement for the repository renderer and semantic tests.
