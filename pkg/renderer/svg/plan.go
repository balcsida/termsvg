package svg

import (
	"slices"
	"sort"
	"time"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type renderPlan struct {
	duration          time.Duration
	staticRows        []ir.Row
	content           timeline[[]ir.Row]
	cursor            timeline[ir.Cursor]
	cursorEverVisible bool
	usedColors        []color.ID
}

func buildRenderPlan(rec *ir.Recording, showCursor bool) renderPlan {
	plan := renderPlan{duration: rec.Duration}
	content := make([]timelinePoint[[]ir.Row], 0, len(rec.Frames))
	for _, frame := range rec.Frames {
		content = append(content, timelinePoint[[]ir.Row]{time: frame.Time, state: frame.Rows})
	}
	plan.content = normalizeTimeline(rec.Duration, content, rowsEqual)
	plan.hoistStaticRows(rec.Height)

	if showCursor {
		cursor := make([]timelinePoint[ir.Cursor], 0, len(rec.Frames))
		for _, frame := range rec.Frames {
			cursor = append(cursor, timelinePoint[ir.Cursor]{time: frame.Time, state: frame.Cursor})
		}
		plan.cursor = normalizeTimeline(rec.Duration, cursor, func(a, b ir.Cursor) bool { return a == b })
		plan.cursorEverVisible = slices.ContainsFunc(plan.cursor.points, func(point timelinePoint[ir.Cursor]) bool {
			return point.state.Visible
		})
		if !plan.cursorEverVisible {
			plan.cursor.points = nil
		}
	}
	plan.usedColors = usedColorIDs(rec, plan.staticRows, plan.content.points)
	return plan
}

func (p *renderPlan) hoistStaticRows(height int) {
	if len(p.content.points) == 0 {
		return
	}
	static := make(map[int]bool, height)
	for y := range height {
		first := rowAt(p.content.points[0].state, y)
		static[y] = true
		for _, point := range p.content.points[1:] {
			if !rowEqual(first, rowAt(point.state, y)) {
				static[y] = false
				break
			}
		}
		if static[y] && len(first.Runs) > 0 {
			p.staticRows = append(p.staticRows, first)
		}
	}
	for i := range p.content.points {
		rows := make([]ir.Row, 0, len(p.content.points[i].state))
		for _, row := range p.content.points[i].state {
			if !static[row.Y] {
				rows = append(rows, row)
			}
		}
		p.content.points[i].state = rows
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

func usedColorIDs(rec *ir.Recording, staticRows []ir.Row, points []timelinePoint[[]ir.Row]) []color.ID {
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
	for _, point := range points {
		visit(point.state)
	}
	ids := make([]color.ID, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
