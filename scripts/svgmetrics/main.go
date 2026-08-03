// Command svgmetrics reports deterministic size and structure metrics for SVG files.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

var (
	translateRE = regexp.MustCompile(`(?i)translate(?:x|y|3d)?\(([^)]*)\)`)
	numberRE    = regexp.MustCompile(`[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`)
)

type metrics struct {
	RawBytes, MinifiedBytes, GzipBytes                 int
	Elements, FilterRefs, Keyframes, KeyframeSelectors int
	DuplicateSelectors, AnimatedGroups                 int
	MaxTranslate                                       float64
	Tags                                               map[string]int
}

type cssToken struct {
	typeID css.TokenType
	text   string
}

type cssMetrics struct {
	filters, keyframes, selectors, duplicates int
	animatedClasses                           map[string]bool
}

type groupInfo struct {
	classes         []string
	inlineAnimation bool
	smilAnimation   bool
}

func measure(raw, minified []byte) (metrics, error) {
	result := metrics{RawBytes: len(raw), MinifiedBytes: len(minified), Tags: map[string]int{}}
	if err := validXML(minified); err != nil {
		return result, fmt.Errorf("minified SVG: %w", err)
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(minified); err != nil {
		return result, err
	}
	if err := zw.Close(); err != nil {
		return result, err
	}
	result.GzipBytes = compressed.Len()

	var styles, transforms []string
	var groups []groupInfo
	var parents []int
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			result.Elements++
			result.Tags[token.Name.Local]++
			parent := -1
			if len(parents) > 0 {
				parent = parents[len(parents)-1]
			}
			if token.Name.Local == "g" {
				parent = len(groups)
				groups = append(groups, groupInfo{})
			}
			parents = append(parents, parent)
			for _, attr := range token.Attr {
				switch attr.Name.Local {
				case "filter":
					result.FilterRefs++
				case "class":
					if token.Name.Local == "g" {
						groups[parent].classes = strings.Fields(attr.Value)
					}
				case "style":
					analysis, err := analyzeDeclarations(attr.Value)
					if err != nil {
						return result, err
					}
					result.FilterRefs += analysis.filters
					if token.Name.Local == "g" && analysis.animated {
						groups[parent].inlineAnimation = true
					}
					transforms = append(transforms, attr.Value)
				case "transform":
					transforms = append(transforms, attr.Value)
				}
			}
			if (token.Name.Local == "animate" || token.Name.Local == "animateTransform" || token.Name.Local == "animateMotion") && parent >= 0 {
				groups[parent].smilAnimation = true
			}
			if token.Name.Local == "style" {
				var text string
				if err := decoder.DecodeElement(&text, &token); err != nil {
					return result, err
				}
				parents = parents[:len(parents)-1]
				styles = append(styles, text)
				transforms = append(transforms, text)
			}
		case xml.EndElement:
			parents = parents[:len(parents)-1]
		}
	}

	animatedClasses := map[string]bool{}
	for _, text := range styles {
		m, err := analyzeCSS(text)
		if err != nil {
			return result, err
		}
		result.FilterRefs += m.filters
		result.Keyframes += m.keyframes
		result.KeyframeSelectors += m.selectors
		result.DuplicateSelectors += m.duplicates
		for class := range m.animatedClasses {
			animatedClasses[class] = true
		}
	}
	for _, group := range groups {
		animated := group.inlineAnimation || group.smilAnimation
		for _, class := range group.classes {
			animated = animated || animatedClasses[class]
		}
		if animated {
			result.AnimatedGroups++
		}
	}
	for _, text := range transforms {
		for _, call := range translateRE.FindAllStringSubmatch(text, -1) {
			for _, number := range numberRE.FindAllString(call[1], -1) {
				value, err := strconv.ParseFloat(number, 64)
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
		}
	}
	return result, nil
}

func validXML(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func lexCSS(text string) ([]cssToken, error) {
	lexer := css.NewLexer(parse.NewInputString(text))
	var tokens []cssToken
	depth := 0
	for {
		typeID, data := lexer.Next()
		if typeID == css.ErrorToken {
			if lexer.Err() == io.EOF {
				break
			}
			return nil, lexer.Err()
		}
		if typeID == css.WhitespaceToken || typeID == css.CommentToken {
			continue
		}
		if typeID == css.LeftBraceToken {
			depth++
		}
		if typeID == css.RightBraceToken {
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced CSS braces")
			}
		}
		tokens = append(tokens, cssToken{typeID, string(data)})
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced CSS braces")
	}
	return tokens, nil
}

func analyzeCSS(text string) (cssMetrics, error) {
	tokens, err := lexCSS(text)
	if err != nil {
		return cssMetrics{}, err
	}
	result := cssMetrics{animatedClasses: map[string]bool{}}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].typeID == css.IdentToken && strings.EqualFold(tokens[i].text, "filter") && i+1 < len(tokens) && tokens[i+1].typeID == css.ColonToken {
			result.filters++
		}
		if tokens[i].typeID != css.AtKeywordToken || !strings.EqualFold(tokens[i].text, "@keyframes") {
			continue
		}
		result.keyframes++
		for i < len(tokens) && tokens[i].typeID != css.LeftBraceToken {
			i++
		}
		if i == len(tokens) {
			return result, fmt.Errorf("malformed @keyframes")
		}
		depth, seen := 1, map[string]bool{}
		for i++; i < len(tokens) && depth > 0; i++ {
			token := tokens[i]
			if depth == 1 {
				selector := ""
				if token.typeID == css.PercentageToken {
					value, err := strconv.ParseFloat(strings.TrimSuffix(token.text, "%"), 64)
					if err != nil {
						return result, err
					}
					selector = strconv.FormatFloat(value, 'g', -1, 64) + "%"
				} else if token.typeID == css.IdentToken && strings.EqualFold(token.text, "from") {
					selector = "0%"
				} else if token.typeID == css.IdentToken && strings.EqualFold(token.text, "to") {
					selector = "100%"
				}
				if selector != "" {
					result.selectors++
					if seen[selector] {
						result.duplicates++
					}
					seen[selector] = true
				}
			}
			if token.typeID == css.LeftBraceToken {
				depth++
			} else if token.typeID == css.RightBraceToken {
				depth--
			}
		}
		i--
	}

	for start := 0; start < len(tokens); {
		open := start
		for open < len(tokens) && tokens[open].typeID != css.LeftBraceToken {
			open++
		}
		if open == len(tokens) {
			break
		}
		depth, end := 1, open+1
		for ; end < len(tokens) && depth > 0; end++ {
			if tokens[end].typeID == css.LeftBraceToken {
				depth++
			} else if tokens[end].typeID == css.RightBraceToken {
				depth--
			}
		}
		animated := false
		for i := open + 1; i+1 < end-1; i++ {
			animated = animated || (tokens[i].typeID == css.IdentToken && strings.HasPrefix(strings.ToLower(tokens[i].text), "animation") && tokens[i+1].typeID == css.ColonToken)
		}
		if animated {
			for i := start; i+1 < open; i++ {
				if tokens[i].typeID == css.DelimToken && tokens[i].text == "." && tokens[i+1].typeID == css.IdentToken {
					result.animatedClasses[tokens[i+1].text] = true
				}
			}
		}
		start = end
	}
	return result, nil
}

type declarationMetrics struct {
	filters  int
	animated bool
}

func analyzeDeclarations(text string) (declarationMetrics, error) {
	tokens, err := lexCSS(text)
	if err != nil {
		return declarationMetrics{}, err
	}
	var result declarationMetrics
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].typeID != css.IdentToken || tokens[i+1].typeID != css.ColonToken {
			continue
		}
		name := strings.ToLower(tokens[i].text)
		if name == "filter" {
			result.filters++
		}
		result.animated = result.animated || strings.HasPrefix(name, "animation")
	}
	return result, nil
}

type pairs map[string]string

func (p pairs) String() string { return "raw.svg=minified.svg" }
func (p pairs) Set(value string) error {
	raw, minified, ok := strings.Cut(value, "=")
	if !ok || raw == "" || minified == "" {
		return fmt.Errorf("minified pair must be raw.svg=minified.svg")
	}
	p[raw] = minified
	return nil
}

func run(args []string, stdout io.Writer) error {
	set := flag.NewFlagSet("svgmetrics", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	minifiedFiles := pairs{}
	set.Var(minifiedFiles, "minified", "raw.svg=minified.svg (repeatable)")
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() == 0 {
		return fmt.Errorf("usage: svgmetrics -minified raw.svg=minified.svg raw.svg [...]")
	}
	records := [][]string{{"file", "minified_file", "raw_bytes", "minified_bytes", "gzip_bytes", "elements", "tags", "filter_references", "keyframes", "keyframe_selectors", "duplicate_selectors", "max_translate", "use", "animated_groups"}}
	for _, name := range set.Args() {
		minifiedName, ok := minifiedFiles[name]
		if !ok {
			return fmt.Errorf("%s: missing -minified pair", name)
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		minified, err := os.ReadFile(minifiedName)
		if err != nil {
			return err
		}
		m, err := measure(raw, minified)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
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
		records = append(records, []string{name, minifiedName, strconv.Itoa(m.RawBytes), strconv.Itoa(m.MinifiedBytes), strconv.Itoa(m.GzipBytes), strconv.Itoa(m.Elements), strings.Join(counts, ","), strconv.Itoa(m.FilterRefs), strconv.Itoa(m.Keyframes), strconv.Itoa(m.KeyframeSelectors), strconv.Itoa(m.DuplicateSelectors), strconv.FormatFloat(m.MaxTranslate, 'g', -1, 64), strconv.Itoa(m.Tags["use"]), strconv.Itoa(m.AnimatedGroups)})
	}
	var output bytes.Buffer
	w := csv.NewWriter(&output)
	w.Comma = '\t'
	w.WriteAll(records)
	if err := w.Error(); err != nil {
		return err
	}
	_, err := stdout.Write(output.Bytes())
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
