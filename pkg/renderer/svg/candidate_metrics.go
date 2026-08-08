package svg

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

// CandidateMetrics describes the serialized size and structural cost of one SVG candidate.
type CandidateMetrics struct {
	FinalBytes                            int64
	XMLNodes                              int
	DefinitionNodes                       int
	ActiveNodes                           int
	TextNodes                             int
	RectNodes                             int
	GroupNodes                            int
	UseNodes                              int
	AnimationNodes                        int
	AnimatedElements                      int
	StateDefinitions                      int
	MaxUseDepth                           int
	MaxTranslatedWidth                    int64
	MaxTranslatedArea                     int64
	LocalViewportCount                    int
	MaxViewportWidth                      int
	MaxViewportHeight                     int
	MaxViewportArea                       int64
	SourceActiveNodes                     int
	SourceDefinitionNodes                 int
	StaticUseShadowNodes                  uint64
	InitialAnimatedUseShadowNodes         uint64
	PeakAnimatedUseShadowNodes            uint64
	PeakInstantiatedNodes                 uint64
	DurationWeightedInstantiatedNodeNanos uint64
	UseTargetSwitches                     uint64
	PeakLiveNodeEstimate                  uint64
}

func addPreparedMetrics(
	metrics *CandidateMetrics,
	content *preparedContent,
	options Options,
	contentWidth, contentHeight int,
) {
	metrics.StateDefinitions = len(content.frameStateIDs)
	for i := range content.bands {
		band := &content.bands[i]
		metrics.StateDefinitions += len(band.stateIDs)
		metrics.LocalViewportCount++
		metrics.MaxViewportWidth = max(metrics.MaxViewportWidth, band.width*ColWidth)
		metrics.MaxViewportHeight = max(metrics.MaxViewportHeight, band.height*RowHeight)
		metrics.MaxViewportArea = max(metrics.MaxViewportArea, int64(band.width*ColWidth)*int64(band.height*RowHeight))
		if options.FrameSwitch == FrameSwitchTranslate && len(band.keyframes) > 1 {
			addTranslatedSurface(metrics, band.width*ColWidth, band.height*RowHeight, len(band.rows))
		}
	}
	if !options.usesLocalViewports() && options.FrameSwitch == FrameSwitchTranslate && len(content.frameKeyframes) > 1 {
		addTranslatedSurface(metrics, contentWidth, contentHeight, len(content.frameRows))
	}
	if len(content.rowDefs) > 0 {
		metrics.MaxUseDepth = 1
	}
	if metrics.StateDefinitions > 0 {
		metrics.MaxUseDepth++
	}
}

func addTranslatedSurface(metrics *CandidateMetrics, width, height, states int) {
	translatedWidth := int64(width * states)
	metrics.MaxTranslatedWidth = max(metrics.MaxTranslatedWidth, translatedWidth)
	metrics.MaxTranslatedArea = max(metrics.MaxTranslatedArea, translatedWidth*int64(height))
}

func addStructuralMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) error {
	addMetric(metrics, false, "svg", 1)
	addMetric(metrics, false, "rect", 1)
	if c.config.ShowWindow {
		addMetric(metrics, false, "circle", len(c.config.Theme.WindowButtons))
	}
	addMetric(metrics, true, "defs", 1)
	addMetric(metrics, true, "clipPath", 1)
	addMetric(metrics, true, "rect", 1)
	addMetric(metrics, false, "g", 1)
	addMetric(metrics, false, "style", 1)

	for _, definition := range content.rowDefs {
		if c.rowElementCount(definition.row) > 1 {
			addMetric(metrics, true, "g", 1)
		}
		renderedDefinition := *definition
		renderedDefinition.id = ""
		addRowMetrics(metrics, c, true, &renderedDefinition)
	}
	for _, static := range c.plan.staticRows {
		addRowMetrics(metrics, c, false, &renderedRow{row: static})
	}

	for i := range content.frameStateIDs {
		if c.stateElementCount(content.frameRows[i]) != 1 {
			addMetric(metrics, true, "g", 1)
		}
		addStateRowsMetrics(metrics, c, true, content.frameRows[i:i+1])
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		for i := range band.stateIDs {
			if c.stateElementCount(band.rows[i]) != 1 {
				addMetric(metrics, true, "g", 1)
			}
			addStateRowsMetrics(metrics, c, true, band.rows[i:i+1])
		}
	}

	animations := addActiveContentMetrics(metrics, c, content)
	if c.plan.cursorEverVisible {
		addMetric(metrics, false, "g", 1)
		addMetric(metrics, false, "rect", 1)
		animations++ // The cursor rect always has the CSS blink animation.
		if c.options.Animation == AnimationCSS && len(c.cursorKeyframes()) > 1 {
			animations++
		}
		if c.options.Animation == AnimationSMIL && len(c.cursorKeyframes()) > 1 {
			cursorAnimations := cursorAnimationCount(c.cursorKeyframes())
			addMetric(metrics, false, "animation", cursorAnimations)
			if cursorAnimations > 0 {
				animations++
			}
		}
	}
	metrics.AnimatedElements = animations
	return addUseExpansionMetrics(metrics, c, content)
}

type definitionNode struct {
	nodes uint64
	uses  []string
}

func recursiveDefinitionCost(graph map[string]definitionNode, id string, visiting map[string]bool) (uint64, int, error) {
	if visiting[id] {
		return 0, 0, fmt.Errorf("cyclic SVG use definition %q", id)
	}
	node, ok := graph[id]
	if !ok {
		return 0, 0, fmt.Errorf("unknown SVG use definition %q", id)
	}
	visiting[id] = true
	cost, depth := node.nodes, 1
	for _, target := range node.uses {
		nested, nestedDepth, err := recursiveDefinitionCost(graph, target, visiting)
		if err != nil {
			return 0, 0, err
		}
		cost = saturatingAdd(cost, nested)
		depth = max(depth, nestedDepth+1)
	}
	delete(visiting, id)
	return cost, depth, nil
}

func saturatingAdd(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}
func saturatingMul(a, b uint64) uint64 {
	if a != 0 && b > math.MaxUint64/a {
		return math.MaxUint64
	}
	return a * b
}

func addUseExpansionMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) error {
	metrics.SourceActiveNodes, metrics.SourceDefinitionNodes = metrics.ActiveNodes, metrics.DefinitionNodes
	graph := make(map[string]definitionNode)
	for _, row := range content.rowDefs {
		graph[row.id] = definitionNode{nodes: uint64(c.rowElementCount(row.row) + boolInt(c.rowElementCount(row.row) > 1))}
	}
	rowUses := func(rows []*renderedRow) (nodes uint64, ids []string) {
		for _, row := range rows {
			if row.id == "" {
				nodes = saturatingAdd(nodes, uint64(c.rowElementCount(row.row)))
				continue
			}
			ids = append(ids, row.id)
		}
		return
	}
	for i, id := range content.frameStateIDs {
		nodes, uses := rowUses(content.frameRows[i])
		graph[id] = definitionNode{nodes: max(nodes, 1), uses: uses}
	}
	for bi := range content.bands {
		for i, id := range content.bands[bi].stateIDs {
			nodes, uses := rowUses(content.bands[bi].rows[i])
			graph[id] = definitionNode{nodes: max(nodes, 1), uses: uses}
		}
	}
	costs := make(map[string]uint64, len(graph))
	ids := make([]string, 0, len(graph))
	for id := range graph {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		cost, depth, err := recursiveDefinitionCost(graph, id, map[string]bool{})
		if err != nil {
			return err
		}
		costs[id] = cost
		metrics.MaxUseDepth = max(metrics.MaxUseDepth, depth)
	}
	expand := func(id string) uint64 { return costs[id] }
	definitionShadow := uint64(0)
	for _, node := range graph {
		for _, id := range node.uses {
			definitionShadow = saturatingAdd(definitionShadow, expand(id))
		}
	}
	metrics.StaticUseShadowNodes = definitionShadow
	initial, peak, weighted, switches := uint64(0), uint64(0), uint64(0), uint64(0)
	if c.options.FrameSwitch == FrameSwitchHref {
		series := make([]hrefMetricSeries, 0, len(content.bands)+1)
		if len(content.frameStateIDs) > 0 {
			series = append(series, newHrefMetricSeries(content.frameKeyframes, content.frameStateIDs, c.plan.duration, expand))
		}
		for i := range content.bands {
			band := &content.bands[i]
			if len(band.stateIDs) > 0 {
				series = append(series, newHrefMetricSeries(band.keyframes, band.stateIDs, c.plan.duration, expand))
			}
		}
		initial, peak, weighted, switches = combineHrefMetricSeries(series, c.plan.duration)
	} else {
		for _, states := range append([][][]*renderedRow{content.frameRows}, bandRows(content.bands)...) {
			for _, rows := range states {
				for _, row := range rows {
					if row.id != "" {
						peak = saturatingAdd(peak, expand(row.id))
					}
				}
			}
		}
		initial = peak
		weighted = saturatingMul(peak, uint64(max(time.Duration(0), c.plan.duration)))
	}
	metrics.InitialAnimatedUseShadowNodes, metrics.PeakAnimatedUseShadowNodes = initial, peak
	metrics.UseTargetSwitches = switches
	shadows := saturatingAdd(definitionShadow, peak)
	metrics.PeakInstantiatedNodes = saturatingAdd(uint64(metrics.XMLNodes), shadows)
	metrics.PeakLiveNodeEstimate = saturatingAdd(uint64(metrics.ActiveNodes), peak)
	duration := uint64(max(time.Duration(0), c.plan.duration))
	base := saturatingAdd(uint64(metrics.XMLNodes), definitionShadow)
	metrics.DurationWeightedInstantiatedNodeNanos = saturatingAdd(saturatingMul(base, duration), weighted)
	return nil
}

type hrefMetricPoint struct {
	time time.Duration
	cost uint64
}
type hrefMetricSeries struct {
	points   []hrefMetricPoint
	switches uint64
}

func newHrefMetricSeries(frames []keyframePoint[int], ids []string, duration time.Duration, expand func(string) uint64) hrefMetricSeries {
	series := hrefMetricSeries{points: make([]hrefMetricPoint, 0, len(frames))}
	lastState := -1
	for _, frame := range frames {
		if frame.state < 0 || frame.state >= len(ids) {
			continue
		}
		if lastState >= 0 && frame.state != lastState {
			series.switches++
		}
		lastState = frame.state
		series.points = append(series.points, hrefMetricPoint{time: keyframeSelectorTime(frame.selector, duration), cost: expand(ids[frame.state])})
	}
	return series
}

func combineHrefMetricSeries(series []hrefMetricSeries, duration time.Duration) (initial, peak, weighted, switches uint64) {
	boundaries := []time.Duration{0, duration}
	for _, sequence := range series {
		switches = saturatingAdd(switches, sequence.switches)
		for _, point := range sequence.points {
			boundaries = append(boundaries, point.time)
		}
	}
	slices.Sort(boundaries)
	boundaries = slices.Compact(boundaries)
	costAt := func(sequence hrefMetricSeries, at time.Duration) uint64 {
		cost := uint64(0)
		for _, point := range sequence.points {
			if point.time > at {
				break
			}
			cost = point.cost
		}
		return cost
	}
	for i, boundary := range boundaries {
		live := uint64(0)
		for _, sequence := range series {
			live = saturatingAdd(live, costAt(sequence, boundary))
		}
		if i == 0 {
			initial = live
		}
		peak = max(peak, live)
		if i+1 < len(boundaries) {
			weighted = saturatingAdd(weighted, saturatingMul(live, uint64(max(time.Duration(0), boundaries[i+1]-boundary))))
		}
	}
	return
}

func keyframeSelectorTime(selector string, duration time.Duration) time.Duration {
	decimal := strings.TrimSuffix(selector, "%")
	parts := strings.SplitN(decimal, ".", 2)
	digits, scale := parts[0], int64(1)
	if len(parts) == 2 {
		digits += parts[1]
		for range len(parts[1]) {
			scale *= 10
		}
	}
	percent, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(int64(duration)), big.NewInt(percent))
	numerator.Quo(numerator, big.NewInt(100*scale))
	return time.Duration(numerator.Int64())
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func bandRows(bands []preparedBand) [][][]*renderedRow {
	out := make([][][]*renderedRow, len(bands))
	for i := range bands {
		out[i] = bands[i].rows
	}
	return out
}

func addMetric(metrics *CandidateMetrics, definition bool, kind string, count int) {
	if count == 0 {
		return
	}
	metrics.XMLNodes += count
	if definition {
		metrics.DefinitionNodes += count
	} else {
		metrics.ActiveNodes += count
	}
	switch kind {
	case "text":
		metrics.TextNodes += count
	case "rect":
		metrics.RectNodes += count
	case "g":
		metrics.GroupNodes += count
	case "use":
		metrics.UseNodes += count
	case "animation":
		metrics.AnimationNodes += count
	}
}

func addRowMetrics(metrics *CandidateMetrics, c *canvas, definition bool, rendered *renderedRow) {
	if rendered.id != "" {
		addMetric(metrics, definition, "use", 1)
		return
	}
	addMetric(metrics, definition, "rect", len(c.backgroundSpans(rendered.row)))
	for _, run := range rendered.row.Runs {
		if shouldRenderText(run) {
			addMetric(metrics, definition, "text", 1)
		}
	}
}

func addStateRowsMetrics(
	metrics *CandidateMetrics,
	c *canvas,
	definition bool,
	states [][]*renderedRow,
) {
	for _, rows := range states {
		for _, rendered := range rows {
			addRowMetrics(metrics, c, definition, rendered)
		}
	}
}

func addActiveContentMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	switch {
	case c.options.usesLocalViewports():
		return addLocalViewportMetrics(metrics, c, content)
	case len(content.frameKeyframes) <= 1:
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return 0
	case c.options.FrameSwitch == FrameSwitchHref:
		addMetric(metrics, false, "use", 1)
		addMetric(metrics, false, "animation", 1)
		return 1
	default:
		addMetric(metrics, false, "g", 1+len(content.frameRows))
		animations := 1
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
		}
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return animations
	}
}

func addLocalViewportMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	animations := 0
	for i := range content.bands {
		band := &content.bands[i]
		addMetric(metrics, false, "svg", 1)
		if len(band.keyframes) <= 1 {
			addStateRowsMetrics(metrics, c, false, band.rows)
			continue
		}
		if c.options.FrameSwitch == FrameSwitchHref {
			addMetric(metrics, false, "use", 1)
			addMetric(metrics, false, "animation", 1)
			animations++
			continue
		}
		addMetric(metrics, false, "g", 1+len(band.rows))
		animations++
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
		}
		addStateRowsMetrics(metrics, c, false, band.rows)
	}
	return animations
}

func cursorAnimationCount(frames []keyframePoint[ir.Cursor]) int {
	position, visibility := false, false
	for i := 1; i < len(frames); i++ {
		position = position || frames[i-1].state.Col != frames[i].state.Col ||
			frames[i-1].state.Row != frames[i].state.Row
		visibility = visibility || frames[i-1].state.Visible != frames[i].state.Visible
	}
	count := 0
	if position {
		count++
	}
	if visibility {
		count++
	}
	return count
}
