package svg

import (
	"slices"

	"github.com/mrmarble/termsvg/pkg/ir"
)

type rowLane struct {
	y       int
	content timeline[ir.Row]
}

type rowBand struct {
	y       int
	content timeline[[]ir.Row]
}

func buildRowBands(plan *renderPlan, height int) []rowBand {
	lanes := make([]rowLane, 0, height)
	for y := range height {
		points := make([]timelinePoint[ir.Row], len(plan.content.points))
		for i, point := range plan.content.points {
			points[i] = timelinePoint[ir.Row]{time: point.time, state: rowAt(point.state, y)}
		}
		content := normalizeTimeline(plan.duration, points, rowEqual)
		if len(content.points) > 1 {
			lanes = append(lanes, rowLane{y: y, content: content})
		}
	}

	bands := make([]rowBand, 0, len(lanes))
	for start := 0; start < len(lanes); {
		end := start + 1
		for end < len(lanes) && lanes[end].y == lanes[end-1].y+1 &&
			timelineTimesEqual(lanes[start].content, lanes[end].content) {
			end++
		}
		bands = append(bands, combineRowLanes(lanes[start:end]))
		start = end
	}
	return bands
}

func timelineTimesEqual[T, U any](a timeline[T], b timeline[U]) bool {
	return a.duration == b.duration && len(a.points) == len(b.points) &&
		slices.EqualFunc(a.points, b.points, func(a timelinePoint[T], b timelinePoint[U]) bool {
			return a.time == b.time
		})
}

func combineRowLanes(lanes []rowLane) rowBand {
	startY := lanes[0].y
	points := make([]timelinePoint[[]ir.Row], len(lanes[0].content.points))
	for i, point := range lanes[0].content.points {
		rows := make([]ir.Row, 0, len(lanes))
		for _, lane := range lanes {
			row := lane.content.points[i].state
			if len(row.Runs) == 0 {
				continue
			}
			row.Y = lane.y - startY
			rows = append(rows, row)
		}
		points[i] = timelinePoint[[]ir.Row]{time: point.time, state: rows}
	}
	return rowBand{
		y: startY,
		content: timeline[[]ir.Row]{
			duration: lanes[0].content.duration,
			points:   points,
		},
	}
}
