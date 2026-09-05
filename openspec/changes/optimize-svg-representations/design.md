## Context

The current renderer shares an immutable semantic plan, prepares candidates, computes final-byte and structural costs, and serializes the selected candidate. `prepareSelectedCandidate` in `pkg/renderer/svg/renderer.go` compares frames, bands, and regions while keeping animation and switching fixed. Scroll preparation already has proven-scroll detection and snapshot fallback. `runtimeCandidateLess` is a deterministic structural comparator, not browser telemetry.

`contentKeyframesFor` in `content.go` allocates a new state whenever the immediately preceding state differs. Consequently A/B/A creates three state slots. `collectRowsWithHash` already demonstrates collision-safe semantic hashing and equality, but shares whole rows rather than recurring complete states. Prefix splitting is not implemented and would change text-node boundaries.

Existing tests cover exact costs, stable ties, plan immutability, semantic parity, cursor endpoints, wide glyphs, SMIL hrefs, and scrolling. The browser HTML fixture exercises embedding and host styling but is not a playback benchmark. There are no existing main OpenSpec specs.

## Goals / Non-Goals

**Goals:**
- Try broader automatic selection and recurring-state reuse with measurable final-byte and runtime outcomes.
- Evaluate prefix reuse without committing to a production abstraction or feature flag before it proves useful.
- Keep explicit settings, visible states, timestamps, cursor behavior, finite/infinite playback, and immutable input intact.
- Make measurements repeatable and separate observed browser results from structural proxies.

**Non-Goals:**
- Compression testing or implementation of any kind, including existing tooling paths that invoke gzip or Brotli.
- Changing default layout, animation, switching, style, primitive mode, or FPS; adding embedded scripting, fonts, rasterization, or production dependencies.
- A general graph optimizer, per-region switching portfolio, new retained primitives, or broader scrolling detection.

## Decisions

### 1. Establish an uncompressed baseline before editing the renderer

Use the current source revision, not the prebuilt binary. Cover 256colors, 444816, its borderless variant, htop, rgb, and session using both their current Taskfile recipes and controlled same-options comparisons. Keep FPS identical within a comparison; include uncapped semantic checks separately. Do not invoke the existing `svgmetrics` compression path. Use `MeasureCandidate`, actual final file lengths, and narrowly scoped measurement commands instead.

Capture final uncompressed bytes; source/definition and estimated instantiated nodes; animation count; viewport and translated area; export elapsed time and allocations. Run existing renderer tests first, then add focused failing cases for the proposed behavior. Temporary outputs belong outside tracked examples.

For browser checks, record browser version, OS, viewport, font, embedding mode, and background-tab state. Test standalone and image embedding, plus object/inline fidelity where supported. Measure baseline and candidate in alternating order with one warmup and at least five measured runs; report median load/first-visible-content time, playback p95 frame interval, missed-frame rate, and memory where the browser exposes a meaningful metric. A host page's requestAnimationFrame alone does not prove that an embedded SVG paints smoothly; use browser traces/paint evidence and label any proxy.

Use Chromium and Safari where available. Record unavailable engines or memory metrics as unmeasured, never as passed. Observe identical playback intervals, include dense transitions, and inspect finite endpoints and loop boundaries. Recheck reproducible regressions exceeding 5% with additional paired runs before attributing them to a candidate. Promote a profile only with smaller final bytes and no reproducible material loading/playback/memory regression on tested targets. Unresolved regressions leave the current profile in place.

### 2. Expand the existing candidate loop, preserving opt-in boundaries

Add scroll after the existing frames/bands/regions order in `LayoutAuto`, retaining the old stable tie preference. A new `--svg-frame-switch=auto` / `FrameSwitchAuto` explicitly authorizes comparison of translation and href under SMIL; reject it with CSS. With a concrete layout, switching auto compares only that layout's two switching modes. With layout auto, it compares the four eligible layouts under both switching modes. Existing concrete translation/href requests remain fixed. Animation, style, FPS, and primitives remain fixed; do not silently enable rectangle tracks or broaden their current option validation.

Reuse one semantic plan and existing preparation/cost logic. Evaluate the small fixed candidate set in deterministic order; release losing candidate state when practical and serialize the winner once. Do not add concurrency or a new search framework. Exact size selection retains the old candidate set as a baseline, so the expanded search cannot choose a larger result for the same options and size objective. Runtime selection keeps its documented structural comparator; browser results inform recommendations, not platform-dependent output bytes or an invented universal score.

Treat scroll eligibility conservatively. Its finite-loop fill behavior differs in the current implementation, so compare start, transition, end, and post-end semantics before admitting it to auto for each playback configuration. Where equivalence is not established, omit scroll from auto rather than change established explicit-layout behavior. Debug output must identify layout, switching mode, objective, costs, and winner.

### 3. Intern recurring complete states within each timeline

Map each normalized content keyframe to a unique state in first-occurrence order. Reuse semantic row hashing with full ordered-row equality to resolve collisions. Preserve every selector and duration: A/B/A becomes state indices 0/1/0, not a shorter timeline. Include empty states, local coordinates, attributes, and glyph extents in equality. Keep pools local to a frame or band/region timeline; cross-viewport sharing is deferred.

Trace all callers of `contentKeyframesFor`, including scroll/retained-track preparation, before editing it. Apply the same index mapping to translated strips, href definitions, and dependent cost/structural calculations. Do not introduce a nested use layer. Deduplication changes row-use counts and thus row-definition profitability, so compare complete prepared costs and retain the original representation if the deduplicated candidate is not smaller. Ties keep the baseline. No FPS loss or mutation of IR is allowed.

### 4. Treat text-prefix reuse as a bounded experiment

Start with repeated prefixes at the same position and paint style, especially 444816 and a synthetic progressive-text fixture. Limit the prototype to a demonstrably safe subset and compare a shared-prefix-plus-suffix representation with existing complete text runs. Include prefix definitions, references, suffix placement, styles, and runtime nodes in the cost; count no hypothetical savings.

Splitting text can change shaping, ligatures, kerning, fallback-font metrics, underline appearance, and selection. ASCII or a requested monospace family alone does not establish equivalence. Verify actual glyph placement under the configured fonts and embedding modes, and retain the original text node for unsupported or uncertain cases. Wide/combining glyphs, underlining, and style boundaries require explicit coverage or conservative fallback.

Keep the experiment isolated from production output until it passes the byte, semantic, visual, and browser gates. If no useful safe subset wins, remove prototype code and record a rejected result. Do not leave dead configuration, a generic substring dictionary, or a dormant dependency. If it succeeds, reuse the existing profitability-based preparation path with a complete-state fallback and document the narrowly supported subset.

## Risks / Trade-offs

- More auto candidates increase export CPU and peak memory -> measure against the old selection and explicit winning profile; retain a fixed bounded set and shared plan.
- State interning changes row-reference counts and ID assignment -> compare complete costs, use deterministic first occurrence, and verify measured bytes against serialized bytes.
- Smaller definitions do not guarantee faster playback -> measure browser loading and painting; report structural estimates separately.
- Scroll endpoint semantics can differ -> gate auto eligibility without changing explicit compatibility behavior.
- Prefix reuse can add nodes or alter text shaping -> strict fallback and a removable experiment rather than a promised production feature.
- Existing integration/example commands can rewrite fixtures -> baseline and experiment outputs go to temporary paths; inspect status before and after checks.

## Migration Plan

No migration or default change is required. Implement on a feature branch, with independently reversible commits for measurement support, recurring-state reuse, selection changes, and any accepted prefix optimization. Update CLI help and README for switching auto. Update example recipes only after per-example evidence establishes an improvement; preserve current recipes otherwise. Rollback removes the individual optimization and leaves existing concrete modes available.

## Open Questions

- Which browsers and tracing/memory facilities are available during implementation? Record actual coverage before making runtime claims.
- Does prefix reuse produce any net benefit after safe shaping constraints and reference overhead? A measured rejection completes that experiment.
