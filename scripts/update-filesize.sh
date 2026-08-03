#!/usr/bin/env bash

# Root folder using git
BASE_PATH=$(git rev-parse --show-toplevel)

EXAMPLES_FOLDER="$BASE_PATH/examples"
README="$EXAMPLES_FOLDER/README.md"
SVG_FILES=("$EXAMPLES_FOLDER"/*.svg)

# Markdown table header
TABLE='| File | Iterations | First Size | Current Size | Variation |
|------|:----------:|------------|--------------|-----------|'
TABLE+=$'\n'

format_size() {
    if command -v numfmt >/dev/null 2>&1; then
        numfmt --to=si --suffix=B --format=%.2f "$1"
    else
        awk -v bytes="$1" 'BEGIN {
            split("B kB MB GB TB PB", units)
            for (unit = 1; bytes >= 1000 && unit < 6; unit++) bytes /= 1000
            printf "%.2f%s\n", bytes, units[unit]
        }'
    fi
}

for filepath in "${SVG_FILES[@]}"; do
    [ -e "$filepath" ] || continue
    filename=$(basename "$filepath")

    # Get git commit hash history for file
    blobs=$(git log --all --format=%H --diff-filter=d -- "examples/$filename")

    formated_sizes=()
    raw_sizes=()
    for blob in $blobs; do
        # Join commit hash with file path
        gittag="$blob:examples/$filename"

        # Obtain file size history in bytes
        bytes=$(git cat-file -s "$gittag")
        raw_sizes[${#raw_sizes[@]}]=$bytes

        # Format bytes to human readable sizes
        formated_sizes[${#formated_sizes[@]}]=$(format_size "$bytes")
    done

    [ "${#raw_sizes[@]}" -gt 0 ] || continue
    last_index=$((${#raw_sizes[@]} - 1))
    first=${raw_sizes[$last_index]}
    current=$(wc -c < "$filepath" | tr -d '[:space:]')
    current_formatted=$(format_size "$current")

    # Calculate variation percent using bc to suport floating point
    variation=$(echo "scale=4; (($first-$current)/(($first+$current)/2))*100" | bc)

    # Append row to table
    TABLE+="| $filename | ${#formated_sizes[@]} | ${formated_sizes[$last_index]} | $current_formatted | $variation% |"$'\n'
done

TABLE+=$'\n'

lead='<!--SIZES_START-->'
tail='<!--SIZES_END-->'

# Replace markers with table
tmp_readme=$(mktemp "$README.XXXXXX")
tmp_table=$(mktemp "$README.table.XXXXXX")
trap 'rm -f "$tmp_readme" "$tmp_table"' EXIT
printf '%s' "$TABLE" > "$tmp_table"
awk -v lead="$lead" -v tail="$tail" -v table_file="$tmp_table" '
    $0 == lead {
        found_lead = 1
        in_sizes = 1
        print
        while ((getline line < table_file) > 0) print line
        close(table_file)
        next
    }
    $0 == tail { found_tail = 1; in_sizes = 0 }
    !in_sizes { print }
    END { if (!found_lead || !found_tail) exit 1 }
' "$README" > "$tmp_readme" && mv "$tmp_readme" "$README"
