# Text-prefix reuse experiment: rejected

The one tested representation replaces repeated same-position, same-style ASCII prefixes with a shared `<text>` definition and `<use>`, followed by a suffix positioned at `original x + 12 * prefix length`. Original IDs are preserved with group wrappers. Underlined/unsupported attributes and non-ASCII text use the unchanged baseline. The prototype is isolated at `/private/tmp/termsvg-prefix/prototype.py`; no production code implements it.

## Byte results

- 444816: 259,469 baseline bytes versus 258,558 candidate bytes; 911 bytes saved (0.351%). Only two prefix groups remain profitable after definitions, references, suffix attributes, and ID wrappers: 70-character prefix used 11 times saves 443 bytes; 58-character prefix used 15 times saves 468 bytes.
- Progressive typing: 1,880 bytes before and after. Existing static-content extraction already hoists the stable prefix, leaving no useful prefix-sharing candidate.

These are final uncompressed SVG bytes. No compression was executed or measured.

## Runnable check and visual evidence

The temporary Python helper includes an assert-based check for net savings, valid XML, preserved original IDs, and Unicode/underline fallback. Before the implementation, `python3 /private/tmp/termsvg-prefix/prototype.py` failed with `AssertionError: profitable repeated prefix remains duplicated`. The same command passes with the prototype.

Chrome exact-time screenshots compare start, representative SMIL boundaries, and duration/end boundaries in standalone, object, and inline embeddings. Baseline-versus-baseline control: 42/42 comparisons have zero differing pixels. Prefix candidate: 24/42 comparisons differ, each by 51 pixels; a representative comparison has maximum channel delta 39.

Measured prefix advances are 840.140625 and 696.125 CSS pixels, while the prototype uses offsets 840 and 696. The -0.140625/-0.125-pixel errors alter glyph placement. The configured Monaco stack and tested Courier New/monospace overrides all retain fractional advances. ASCII plus a nominal monospace font is therefore insufficient to establish safe splitting.

Raw evidence is in `/private/tmp/termsvg-measurements/prefix-fidelity/`, `prefix-control/`, and `prefix-geometry.json`. The synthetic recording and unchanged result are under `/private/tmp/termsvg-prefix/`.

## Decision

Reject the representation at the visual-parity gate. A browser runtime benchmark cannot rescue a candidate that changes glyph placement, so it was not run for this prototype. Fixing this would require font-specific measured placement or another representation, beyond the approved bounded experiment and disproportionate to the observed savings. No prototype production change, feature flag, dependency, or dormant abstraction remains.
