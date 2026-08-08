package svg

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type preparedContent struct {
	frameKeyframes []keyframePoint[int]
	frameRows      [][]*renderedRow
	frameStateIDs  []string
	bands          []preparedBand
	rowDefs        []*renderedRow
	cost           preparedContentCost
}

type preparedBand struct {
	kind       preparedBandKind
	x          int
	y          int
	width      int
	height     int
	name       string
	keyframes  []keyframePoint[int]
	rows       [][]*renderedRow
	stateIDs   []string
	content    timeline[[]ir.Row]
	tapeRows   []*renderedRow
	tapeRaw    []ir.Row
	offsets    []keyframePoint[int]
	direction  int
	tapeHeight int
}

type preparedBandKind uint8

const (
	bandSnapshot preparedBandKind = iota
	bandScrollTape
)

func (c *canvas) prepareContentContext(ctx context.Context) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	if c.options.Layout == LayoutBands {
		return c.prepareBands(ctx)
	}
	if c.options.Layout == LayoutRegions {
		return c.prepareRegions(ctx)
	}
	if c.options.Layout == LayoutScroll {
		return c.prepareScroll(ctx)
	}
	keyframes, states := c.contentKeyframes()
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	frames, defs := c.collectRows(states)
	prepared := preparedContent{frameKeyframes: keyframes, frameRows: frames, rowDefs: defs}
	if c.options.FrameSwitch == FrameSwitchHref && len(keyframes) > 1 {
		prepared.frameStateIDs = stateIDs("_f", len(frames))
	}
	prepared.cost = buildPreparedContentCost(c, &prepared)
	return prepared, contextErr(ctx)
}

func (c *canvas) prepareRegions(ctx context.Context) (preparedContent, error) {
	regions := buildDynamicRegions(&c.plan, c.rec.Colors)
	regions, err := c.optimizeDynamicRegions(ctx, regions)
	if err != nil {
		return preparedContent{}, err
	}
	return c.prepareLocalViewports(ctx, c.regionBands(regions))
}

func (c *canvas) regionBands(regions []dynamicRegion) []rowBand {
	bands := make([]rowBand, len(regions))
	for i, region := range regions {
		x, width, content := region.x, region.width, region.content
		if len(region.fallbackRows) == 0 {
			x = max(0, region.x-1)
			end := min(c.plan.width, region.x+region.width+1)
			width = end - x
			content = shiftRegionContent(content, region.x-x)
		}
		bands[i] = rowBand{x: x, y: region.y, width: width, height: region.height, content: content}
	}
	return bands
}

func shiftRegionContent(content timeline[[]ir.Row], columns int) timeline[[]ir.Row] {
	if columns == 0 {
		return content
	}
	shifted := content
	shifted.points = slices.Clone(content.points)
	for pointIndex := range shifted.points {
		shifted.points[pointIndex].state = slices.Clone(content.points[pointIndex].state)
		for rowIndex := range shifted.points[pointIndex].state {
			row := &shifted.points[pointIndex].state[rowIndex]
			row.Runs = slices.Clone(row.Runs)
			for runIndex := range row.Runs {
				row.Runs[runIndex].StartCol += columns
				row.Runs[runIndex].EndCol += columns
			}
		}
	}
	return shifted
}

func (c *canvas) serializedRegionBytes(ctx context.Context, regions []dynamicRegion) (int64, error) {
	content, err := c.prepareLocalViewports(ctx, c.regionBands(regions))
	if err != nil {
		return 0, err
	}
	counter := &countingWriter{}
	if err := writeFinalSVG(counter, c.config.Minify, func(w io.Writer) error {
		return c.renderRegionRepresentation(ctx, w, &content)
	}); err != nil {
		return 0, err
	}
	return counter.size(), nil
}

func (c *canvas) renderRegionRepresentation(ctx context.Context, w io.Writer, content *preparedContent) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	probe := *c
	probe.w = w
	probe.metrics = &CandidateMetrics{}
	probe.plan.cursor = timeline[ir.Cursor]{}
	probe.plan.cursorEverVisible = false
	fmt.Fprint(w, `<svg><defs>`)
	probe.writeRowDefs(content.rowDefs)
	probe.writeStateDefs(content)
	fmt.Fprint(w, `</defs>`)
	probe.writeStyles(content)
	probe.writeContent(content)
	fmt.Fprint(w, `</svg>`)
	return contextErr(ctx)
}

func (c *canvas) prepareBands(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
	return c.prepareLocalViewports(ctx, bands)
}

func (c *canvas) prepareScroll(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
	prepared, err := c.prepareLocalViewports(ctx, bands)
	if err != nil {
		return preparedContent{}, err
	}
	for i, source := range bands {
		tape, ok := detectUpwardScrollTape(source, c.rec.Colors)
		if !ok {
			continue
		}
		candidateBands := slices.Clone(prepared.bands)
		candidateBands[i].kind = bandScrollTape
		candidateBands[i].tapeRaw = tape.rows
		candidateBands[i].offsets = tape.offsets
		candidateBands[i].direction = 1
		candidateBands[i].tapeHeight = len(tape.rows)
		candidate, err := c.materializeBands(ctx, candidateBands)
		if err != nil {
			return preparedContent{}, err
		}
		if scrollTapeWins(prepared.cost, candidate.cost) {
			prepared = candidate
		}
	}
	return prepared, nil
}

// exactBandReplacementDelta is the exact additive change in the complete
// prepared-content ledger when one band is replaced and every other band is
// held fixed. Shared definitions and styles are rebuilt in both ledgers, so
// their attributable byte changes are included without double counting.
func exactBandReplacementDelta(snapshot, tape preparedContentCost) int64 {
	return tape.definitions - snapshot.definitions + tape.styles - snapshot.styles + tape.active - snapshot.active
}

func scrollTapeWins(snapshot, tape preparedContentCost) bool {
	return exactBandReplacementDelta(snapshot, tape) < 0
}

func (c *canvas) prepareLocalViewports(ctx context.Context, bands []rowBand) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared := preparedContent{bands: make([]preparedBand, len(bands))}

	for i, band := range bands {
		keyframes, _ := contentKeyframesFor(band.content)
		prepared.bands[i] = preparedBand{
			kind:      bandSnapshot,
			x:         band.x,
			y:         band.y,
			width:     band.width,
			height:    band.height,
			keyframes: keyframes,
			content:   band.content,
		}
	}
	return c.materializeBands(ctx, prepared.bands)
}

func (c *canvas) materializeBands(ctx context.Context, bands []preparedBand) (preparedContent, error) {
	prepared := preparedContent{bands: slices.Clone(bands)}
	allStates := make([][]ir.Row, 0)
	stateOffsets := make([]int, len(bands)+1)
	for i := range prepared.bands {
		stateOffsets[i] = len(allStates)
		if prepared.bands[i].kind == bandScrollTape {
			allStates = append(allStates, prepared.bands[i].tapeRaw)
			continue
		}
		_, states := contentKeyframesFor(prepared.bands[i].content)
		allStates = append(allStates, states...)
	}
	stateOffsets[len(bands)] = len(allStates)

	frames, defs := c.collectRows(allStates)
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared.rowDefs = defs
	for i := range prepared.bands {
		band := &prepared.bands[i]
		if band.kind == bandScrollTape {
			band.rows = nil
			band.tapeRows = frames[stateOffsets[i]]
			band.stateIDs = nil
			continue
		}
		band.rows = frames[stateOffsets[i]:stateOffsets[i+1]]
		if c.options.FrameSwitch == FrameSwitchHref && len(prepared.bands[i].keyframes) > 1 {
			prepared.bands[i].stateIDs = stateIDs("_b"+strconv.Itoa(i)+"_", len(prepared.bands[i].rows))
		}
	}

	names := make(map[string]string)
	for i := range prepared.bands {
		frames := prepared.bands[i].keyframes
		if prepared.bands[i].kind == bandScrollTape {
			frames = prepared.bands[i].offsets
		}
		if len(frames) <= 1 {
			continue
		}
		signature := strconv.Itoa(int(prepared.bands[i].kind)) + "|" + keyframeSignature(frames, prepared.bands[i].width)
		name, ok := names[signature]
		if !ok {
			name = "b" + strconv.Itoa(len(names))
			names[signature] = name
		}
		prepared.bands[i].name = name
	}
	prepared.cost = buildPreparedContentCost(c, &prepared)
	return prepared, contextErr(ctx)
}

func contentKeyframesFor(content timeline[[]ir.Row]) ([]keyframePoint[int], [][]ir.Row) {
	frames := content.keyframes(rowsEqual)
	states := make([][]ir.Row, 0, len(frames))
	out := make([]keyframePoint[int], len(frames))
	for i, frame := range frames {
		if len(states) == 0 || !rowsEqual(states[len(states)-1], frame.state) {
			states = append(states, frame.state)
		}
		out[i] = keyframePoint[int]{selector: frame.selector, state: len(states) - 1}
	}
	if len(states) == 0 && len(content.points) > 0 {
		states = append(states, content.points[len(content.points)-1].state)
	}
	return out, states
}

func keyframeSignature(frames []keyframePoint[int], width int) string {
	var signature strings.Builder
	signature.WriteString(strconv.Itoa(width))
	signature.WriteByte('|')
	for _, frame := range frames {
		signature.WriteString(frame.selector)
		signature.WriteByte(':')
		signature.WriteString(strconv.Itoa(frame.state))
		signature.WriteByte(';')
	}
	return signature.String()
}

func stateIDs(prefix string, count int) []string {
	ids := make([]string, count)
	for i := range count {
		ids[i] = prefix + strconv.Itoa(i)
	}
	return ids
}
