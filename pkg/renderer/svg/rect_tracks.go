package svg

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type retainedRectTrack struct {
	x, width []keyframePoint[int]
	fill     []keyframePoint[color.ID]
}

func (c *canvas) retainedRectCandidate(band rowBand) (retainedRectTrack, timeline[[]ir.Row], bool) {
	if band.height != 1 {
		return retainedRectTrack{}, timeline[[]ir.Row]{}, false
	}
	keyframes, states := contentKeyframesFor(band.content)
	spans := make([]*backgroundSpan, len(states))
	var first *backgroundSpan
	for i, state := range states {
		if len(state) > 1 {
			return retainedRectTrack{}, timeline[[]ir.Row]{}, false
		}
		var row ir.Row
		if len(state) == 1 {
			row = state[0]
		}
		backgrounds := c.backgroundSpans(row)
		if len(backgrounds) > 1 {
			return retainedRectTrack{}, timeline[[]ir.Row]{}, false
		}
		if len(backgrounds) == 1 {
			span := backgrounds[0]
			spans[i] = &span
			if first == nil {
				first = &span
			}
		}
	}
	if first == nil {
		return retainedRectTrack{}, timeline[[]ir.Row]{}, false
	}
	last := *first
	stateX, stateWidth, stateFill := make([]int, len(states)), make([]int, len(states)), make([]color.ID, len(states))
	for i, span := range spans {
		if span != nil {
			last = *span
			stateWidth[i] = (span.endCol - span.startCol) * ColWidth
		}
		stateX[i] = last.startCol * ColWidth
		stateFill[i] = last.colorID
	}
	track := retainedRectTrack{
		x: make([]keyframePoint[int], len(keyframes)), width: make([]keyframePoint[int], len(keyframes)),
		fill: make([]keyframePoint[color.ID], len(keyframes)),
	}
	for i, frame := range keyframes {
		track.x[i] = keyframePoint[int]{selector: frame.selector, state: stateX[frame.state]}
		track.width[i] = keyframePoint[int]{selector: frame.selector, state: stateWidth[frame.state]}
		track.fill[i] = keyframePoint[color.ID]{selector: frame.selector, state: stateFill[frame.state]}
	}
	return track, stripBackgrounds(band.content), true
}

func stripBackgrounds(content timeline[[]ir.Row]) timeline[[]ir.Row] {
	out := content
	out.points = slices.Clone(content.points)
	for i := range out.points {
		out.points[i].state = slices.Clone(content.points[i].state)
		for j := range out.points[i].state {
			out.points[i].state[j].Runs = slices.Clone(content.points[i].state[j].Runs)
			for k := range out.points[i].state[j].Runs {
				out.points[i].state[j].Runs[k].Attrs.BG = color.DefaultID
			}
		}
	}
	return out
}

func timelineChanges[T comparable](points []keyframePoint[T]) bool {
	for i := 1; i < len(points); i++ {
		if points[i-1].state != points[i].state {
			return true
		}
	}
	return false
}

func (t *retainedRectTrack) geometryAnimationCount() int {
	return boolInt(timelineChanges(t.x)) + boolInt(timelineChanges(t.width))
}

func (t *retainedRectTrack) fillChanges() bool { return timelineChanges(t.fill) }
func (t *retainedRectTrack) animationCount() int {
	return t.geometryAnimationCount() + boolInt(t.fillChanges())
}

func (c *canvas) writeRetainedRect(track *retainedRectTrack) {
	if track == nil || len(track.width) == 0 {
		return
	}
	initialX, initialWidth, initialFill := track.x[0].state, track.width[0].state, track.fill[0].state
	x := ""
	if initialX != 0 {
		x = ` x="` + c.xmlInt(initialX) + `"`
	}
	paint := ` class="` + c.classNames[initialFill] + `"`
	if c.style.scheme != "" && c.style.scheme != styleLegacy {
		paint = styleAttributes(c.style.backgrounds[initialFill])
	}
	fmt.Fprintf(c.w, `<rect%s%s y="0" width="%s" height="%s">`, paint, x, c.xmlInt(initialWidth), c.xmlInt(RowHeight))
	if timelineChanges(track.x) {
		writeRetainedRectAnimation(c, c.w, "x", track.x, func(value int) string { return strconv.Itoa(value) })
	}
	if timelineChanges(track.width) {
		writeRetainedRectAnimation(c, c.w, "width", track.width, func(value int) string { return strconv.Itoa(value) })
	}
	if track.fillChanges() {
		writeRetainedRectAnimation(c, c.w, "fill", track.fill, func(value color.ID) string {
			return c.paintHex(c.rec.Colors.Resolved(value))
		})
	}
	fmt.Fprint(c.w, `</rect>`)
}

func writeRetainedRectAnimation[T any](c *canvas, w io.Writer, name string, points []keyframePoint[T], format func(T) string) {
	values := make([]string, len(points))
	for i, point := range points {
		values[i] = format(point.state)
	}
	c.writeSMILAnimation(w, "animate", []smilAttribute{
		{name: "attributeName", value: name},
		{name: "values", value: strings.Join(values, ";")},
		{name: "keyTimes", value: smilKeyTimes(points)},
	})
}
