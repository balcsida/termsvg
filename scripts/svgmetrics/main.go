// Command svgmetrics reports deterministic size and structure metrics for SVG files.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

var (
	selectorRE  = regexp.MustCompile(`(?:\d+(?:\.\d+)?|\.\d+)%`)
	translateRE = regexp.MustCompile(`translate(?:X)?\(\s*(-?\d+(?:\.\d+)?)`)
)

type metrics struct {
	RawBytes, MinifiedBytes, GzipBytes                  int
	Elements, FilterAttrs, Keyframes, KeyframeSelectors int
	DuplicateSelectors, AnimatedGroups                  int
	MaxTranslate                                        float64
	Tags                                                map[string]int
}

func measure(data []byte) (metrics, error) {
	result := metrics{RawBytes: len(data), Tags: map[string]int{}}
	minifier := minify.New()
	minifier.AddFunc("image/svg+xml", msvg.Minify)
	var minified bytes.Buffer
	if err := minifier.Minify("image/svg+xml", &minified, bytes.NewReader(data)); err != nil {
		return result, err
	}
	result.MinifiedBytes = minified.Len()
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(minified.Bytes()); err != nil {
		return result, err
	}
	if err := zw.Close(); err != nil {
		return result, err
	}
	result.GzipBytes = compressed.Len()

	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		result.Elements++
		result.Tags[start.Name.Local]++
		animated := false
		for _, attr := range start.Attr {
			if attr.Name.Local == "filter" {
				result.FilterAttrs++
			}
			if attr.Name.Local == "style" && strings.Contains(attr.Value, "animation:") {
				animated = true
			}
		}
		if start.Name.Local == "g" && animated {
			result.AnimatedGroups++
		}
	}

	for _, body := range keyframeBodies(string(data)) {
		result.Keyframes++
		seen := map[string]bool{}
		for _, selector := range selectorRE.FindAllString(body, -1) {
			result.KeyframeSelectors++
			if seen[selector] {
				result.DuplicateSelectors++
			}
			seen[selector] = true
		}
	}
	for _, match := range translateRE.FindAllSubmatch(data, -1) {
		value, err := strconv.ParseFloat(string(match[1]), 64)
		if err != nil {
			return result, err
		}
		if value < 0 {
			value = -value
		}
		if value > result.MaxTranslate {
			result.MaxTranslate = value
		}
	}
	return result, nil
}

func keyframeBodies(svg string) []string {
	var bodies []string
	for start := 0; ; {
		i := strings.Index(svg[start:], "@keyframes")
		if i < 0 {
			return bodies
		}
		i += start
		open := strings.IndexByte(svg[i:], '{')
		if open < 0 {
			return bodies
		}
		open += i
		depth, end := 1, open+1
		for ; end < len(svg) && depth > 0; end++ {
			switch svg[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			return bodies
		}
		bodies = append(bodies, svg[open+1:end-1])
		start = end
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/svgmetrics file.svg [...]")
		os.Exit(2)
	}
	fmt.Println("file\traw_bytes\tminified_bytes\tgzip_bytes\telements\ttags\tfilter_attrs\tkeyframes\tkeyframe_selectors\tduplicate_selectors\tmax_translate\tuse\tanimated_groups")
	for _, name := range os.Args[1:] {
		data, err := os.ReadFile(name)
		if err != nil {
			fail(err)
		}
		m, err := measure(data)
		if err != nil {
			fail(fmt.Errorf("%s: %w", name, err))
		}
		keys := make([]string, 0, len(m.Tags))
		for tag := range m.Tags {
			keys = append(keys, tag)
		}
		sort.Strings(keys)
		counts := make([]string, 0, len(keys))
		for _, tag := range keys {
			counts = append(counts, fmt.Sprintf("%s=%d", tag, m.Tags[tag]))
		}
		fmt.Printf("%s\t%d\t%d\t%d\t%d\t%s\t%d\t%d\t%d\t%d\t%g\t%d\t%d\n", filepath.Base(name), m.RawBytes, m.MinifiedBytes, m.GzipBytes, m.Elements, strings.Join(counts, ","), m.FilterAttrs, m.Keyframes, m.KeyframeSelectors, m.DuplicateSelectors, m.MaxTranslate, m.Tags["use"], m.AnimatedGroups)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
