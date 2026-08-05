package svg

import (
	"context"
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
}

type preparedBand struct {
	x         int
	y         int
	width     int
	height    int
	name      string
	keyframes []keyframePoint[int]
	rows      [][]*renderedRow
	stateIDs  []string
	content   timeline[[]ir.Row]
}

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
	keyframes, states := c.contentKeyframes()
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	frames, defs := c.collectRows(states)
	prepared := preparedContent{frameKeyframes: keyframes, frameRows: frames, rowDefs: defs}
	if c.options.FrameSwitch == FrameSwitchHref && len(keyframes) > 1 {
		prepared.frameStateIDs = stateIDs("_f", len(frames))
	}
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
	probe := *c
	probe.w = counter
	probe.metrics = &CandidateMetrics{}
	probe.plan.staticRows = nil
	probe.plan.cursor = timeline[ir.Cursor]{}
	probe.plan.cursorEverVisible = false
	if err := writeFinalSVG(counter, c.config.Minify, func(w io.Writer) error {
		probe.w = w
		return probe.render(ctx, &content)
	}); err != nil {
		return 0, err
	}
	return counter.size(), nil
}

func (c *canvas) prepareBands(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
	return c.prepareLocalViewports(ctx, bands)
}

func (c *canvas) prepareLocalViewports(ctx context.Context, bands []rowBand) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared := preparedContent{bands: make([]preparedBand, len(bands))}
	allStates := make([][]ir.Row, 0)
	stateOffsets := make([]int, len(bands)+1)

	for i, band := range bands {
		keyframes, states := contentKeyframesFor(band.content)
		prepared.bands[i] = preparedBand{
			x:         band.x,
			y:         band.y,
			width:     band.width,
			height:    band.height,
			keyframes: keyframes,
			content:   band.content,
		}
		stateOffsets[i] = len(allStates)
		allStates = append(allStates, states...)
	}
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	stateOffsets[len(bands)] = len(allStates)

	frames, defs := c.collectRows(allStates)
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	prepared.rowDefs = defs
	for i := range prepared.bands {
		prepared.bands[i].rows = frames[stateOffsets[i]:stateOffsets[i+1]]
		if c.options.FrameSwitch == FrameSwitchHref && len(prepared.bands[i].keyframes) > 1 {
			prepared.bands[i].stateIDs = stateIDs("_b"+strconv.Itoa(i)+"_", len(prepared.bands[i].rows))
		}
	}

	names := make(map[string]string)
	for i := range prepared.bands {
		if len(prepared.bands[i].keyframes) <= 1 {
			continue
		}
		signature := keyframeSignature(prepared.bands[i].keyframes, prepared.bands[i].width)
		name, ok := names[signature]
		if !ok {
			name = "b" + strconv.Itoa(len(names))
			names[signature] = name
		}
		prepared.bands[i].name = name
	}
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
