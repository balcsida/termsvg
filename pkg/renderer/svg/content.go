package svg

import (
	"context"
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
}

func (c *canvas) prepareContent() preparedContent {
	content, _ := c.prepareContentContext(context.Background())
	return content
}

func (c *canvas) prepareContentContext(ctx context.Context) (preparedContent, error) {
	if err := contextErr(ctx); err != nil {
		return preparedContent{}, err
	}
	if c.options.Layout == LayoutBands {
		return c.prepareBands(ctx)
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

func (c *canvas) prepareBands(ctx context.Context) (preparedContent, error) {
	bands := buildRowBands(&c.plan, c.plan.width, c.plan.height)
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
