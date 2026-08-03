package svg

import (
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type preparedContent struct {
	frameKeyframes []keyframePoint[int]
	frameRows      [][]*renderedRow
	bands          []preparedBand
	rowDefs        []*renderedRow
}

type preparedBand struct {
	y         int
	name      string
	keyframes []keyframePoint[int]
	rows      [][]*renderedRow
}

func (c *canvas) prepareContent() preparedContent {
	if c.options.Layout == LayoutBands {
		return c.prepareBands()
	}
	keyframes, states := c.contentKeyframes()
	frames, defs := c.collectRows(states)
	return preparedContent{frameKeyframes: keyframes, frameRows: frames, rowDefs: defs}
}

func (c *canvas) prepareBands() preparedContent {
	bands := buildRowBands(&c.plan, c.rec.Height)
	prepared := preparedContent{bands: make([]preparedBand, len(bands))}
	allStates := make([][]ir.Row, 0)
	stateOffsets := make([]int, len(bands)+1)

	for i, band := range bands {
		keyframes, states := contentKeyframesFor(band.content)
		prepared.bands[i] = preparedBand{
			y:         band.y,
			keyframes: keyframes,
		}
		stateOffsets[i] = len(allStates)
		allStates = append(allStates, states...)
	}
	stateOffsets[len(bands)] = len(allStates)

	frames, defs := c.collectRows(allStates)
	prepared.rowDefs = defs
	for i := range prepared.bands {
		prepared.bands[i].rows = frames[stateOffsets[i]:stateOffsets[i+1]]
	}

	names := make(map[string]string)
	for i := range prepared.bands {
		if len(prepared.bands[i].keyframes) <= 1 {
			continue
		}
		signature := keyframeSignature(prepared.bands[i].keyframes)
		name, ok := names[signature]
		if !ok {
			name = "b" + strconv.Itoa(len(names))
			names[signature] = name
		}
		prepared.bands[i].name = name
	}
	return prepared
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

func keyframeSignature(frames []keyframePoint[int]) string {
	var signature strings.Builder
	for _, frame := range frames {
		signature.WriteString(frame.selector)
		signature.WriteByte(':')
		signature.WriteString(strconv.Itoa(frame.state))
		signature.WriteByte(';')
	}
	return signature.String()
}
