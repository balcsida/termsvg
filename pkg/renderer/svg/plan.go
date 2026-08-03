package svg

import (
	"slices"
	"sort"
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type renderPlan struct {
	duration      time.Duration
	staticRows    []ir.Row
	contentFrames []contentFrame
	cursor        cursorPlan
	usedColors    []color.ID
}

type contentFrame struct {
	time time.Duration
	rows []ir.Row
}

type cursorPoint struct {
	time   time.Duration
	cursor ir.Cursor
}

type cursorPlan struct {
	points      []cursorPoint
	everVisible bool
}

func buildRenderPlan(rec *ir.Recording, showCursor bool) renderPlan {
	plan := renderPlan{duration: rec.Duration}
	for _, frame := range rec.Frames {
		if n := len(plan.contentFrames); n > 0 && plan.contentFrames[n-1].time == frame.Time {
			plan.contentFrames[n-1].rows = frame.Rows
		} else {
			plan.contentFrames = append(plan.contentFrames, contentFrame{time: frame.Time, rows: frame.Rows})
		}
	}
	plan.contentFrames = compactContentFrames(plan.contentFrames)
	plan.hoistStaticRows(rec.Height)

	if showCursor {
		for _, frame := range rec.Frames {
			plan.cursor.everVisible = plan.cursor.everVisible || frame.Cursor.Visible
			if n := len(plan.cursor.points); n > 0 && plan.cursor.points[n-1].time == frame.Time {
				plan.cursor.points[n-1].cursor = frame.Cursor
			} else {
				plan.cursor.points = append(plan.cursor.points, cursorPoint{time: frame.Time, cursor: frame.Cursor})
			}
		}
		plan.cursor.points = compactCursorPoints(plan.cursor.points)
		if !plan.cursor.everVisible {
			plan.cursor.points = nil
		}
	}
	plan.usedColors = usedColorIDs(rec, plan.staticRows, plan.contentFrames)
	return plan
}

func compactContentFrames(frames []contentFrame) []contentFrame {
	out := frames[:0]
	for _, frame := range frames {
		if len(out) == 0 || !rowsEqual(out[len(out)-1].rows, frame.rows) {
			out = append(out, frame)
		}
	}
	return out
}

func compactCursorPoints(points []cursorPoint) []cursorPoint {
	out := points[:0]
	for _, point := range points {
		if len(out) == 0 || out[len(out)-1].cursor != point.cursor {
			out = append(out, point)
		}
	}
	return out
}

func (p *renderPlan) hoistStaticRows(height int) {
	if len(p.contentFrames) == 0 {
		return
	}
	static := make(map[int]bool, height)
	for y := range height {
		first := rowAt(p.contentFrames[0].rows, y)
		static[y] = true
		for _, frame := range p.contentFrames[1:] {
			if !rowEqual(first, rowAt(frame.rows, y)) {
				static[y] = false
				break
			}
		}
		if static[y] && len(first.Runs) > 0 {
			p.staticRows = append(p.staticRows, first)
		}
	}
	for i := range p.contentFrames {
		rows := make([]ir.Row, 0, len(p.contentFrames[i].rows))
		for _, row := range p.contentFrames[i].rows {
			if !static[row.Y] {
				rows = append(rows, row)
			}
		}
		p.contentFrames[i].rows = rows
	}
}

func rowsEqual(a, b []ir.Row) bool {
	return slices.EqualFunc(a, b, rowEqual)
}

func rowEqual(a, b ir.Row) bool {
	return a.Y == b.Y && slices.Equal(a.Runs, b.Runs)
}

func rowAt(rows []ir.Row, y int) ir.Row {
	for _, row := range rows {
		if row.Y == y {
			return row
		}
	}
	return ir.Row{Y: y}
}

func usedColorIDs(rec *ir.Recording, staticRows []ir.Row, frames []contentFrame) []color.ID {
	used := make(map[color.ID]bool)
	visit := func(rows []ir.Row) {
		for _, row := range rows {
			for _, run := range row.Runs {
				if !rec.Colors.IsDefault(run.Attrs.BG) && runEndCol(run) > run.StartCol {
					used[run.Attrs.BG] = true
				}
				if shouldRenderText(run) && !rec.Colors.IsDefault(run.Attrs.FG) {
					used[run.Attrs.FG] = true
				}
			}
		}
	}
	visit(staticRows)
	for _, frame := range frames {
		visit(frame.rows)
	}
	ids := make([]color.ID, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
