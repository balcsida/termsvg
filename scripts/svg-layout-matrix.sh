#!/usr/bin/env bash
set -euo pipefail

BASE_PATH=$(git rev-parse --show-toplevel)
TERMSVG=${TERMSVG:-"$BASE_PATH/termsvg"}
CAST=${1:-"$BASE_PATH/examples/444816.cast"}
MAX_FPS=${MAX_FPS:-30}

if [[ ! -x "$TERMSVG" ]]; then
    echo "termsvg binary not found at $TERMSVG; run task build first" >&2
    exit 1
fi
if [[ ! -f "$CAST" ]]; then
    echo "cast file not found: $CAST" >&2
    exit 1
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/termsvg-svg-matrix.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

names=()
raw_files=()
metric_args=()

render_variant() {
    local name=$1
    shift
    local raw="$TMP/$name.raw.svg"
    local minified="$TMP/$name.min.svg"

    "$TERMSVG" export "$CAST" "$@" -o "$raw" >/dev/null
    "$TERMSVG" export "$CAST" "$@" -m -o "$minified" >/dev/null

    names+=("$name")
    raw_files+=("$raw")
    metric_args+=("-minified" "$raw=$minified")
}

common=(--svg-max-fps="$MAX_FPS")
render_variant frames-css-translate "${common[@]}"
render_variant frames-smil-translate "${common[@]}" --svg-animation=smil
render_variant bands-css-translate "${common[@]}" --svg-layout=bands
render_variant bands-smil-translate "${common[@]}" --svg-layout=bands --svg-animation=smil
render_variant frames-smil-href "${common[@]}" --svg-animation=smil --svg-frame-switch=href
render_variant bands-smil-href "${common[@]}" --svg-layout=bands --svg-animation=smil --svg-frame-switch=href
render_variant auto-smil-translate "${common[@]}" --svg-layout=auto --svg-animation=smil
render_variant auto-smil-href "${common[@]}" --svg-layout=auto --svg-animation=smil --svg-frame-switch=href

printf '# cast=%s max_fps=%s\n' "$CAST" "$MAX_FPS"
go run ./scripts/svgmetrics "${metric_args[@]}" "${raw_files[@]}" |
    awk -F '\t' -v OFS='\t' '
        NR == 1 { print "variant", $0; next }
        {
            file = $1
            sub(/^.*\//, "", file)
            sub(/\.raw\.svg$/, "", file)
            print file, $0
        }
    '
