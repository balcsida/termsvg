package svg

import (
	"slices"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

type terminalCell struct {
	char  rune
	attrs ir.CellAttrs
}

// hoistStaticCells extracts cell spans that remain visually identical for the
// complete recording. Whole-row hoisting runs first; this pass handles rows
// where labels, borders, and separators are static around a changing value.
func (p *renderPlan) hoistStaticCells(width, height int, colors *color.Catalog) {
	if width <= 0 || len(p.content.points) < 2 {
		return
	}

	for y := range height {
		states := make([][]terminalCell, len(p.content.points))
		valid := true
		for i, point := range p.content.points {
			var ok bool
			states[i], ok = decodeRowCells(rowAt(point.state, y), width)
			if !ok {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}

		static := make([]bool, width)
		for col := range width {
			first := states[0][col]
			if !cellVisible(first, colors) {
				continue
			}
			static[col] = true
			for _, state := range states[1:] {
				if !cellVisualEqual(first, state[col], colors) {
					static[col] = false
					break
				}
			}
		}
		if !slices.Contains(static, true) {
			continue
		}

		if row := cellsToRow(y, states[0], static, colors); len(row.Runs) > 0 {
			p.staticRows = append(p.staticRows, row)
		}
		for i := range p.content.points {
			keep := make([]bool, width)
			for col := range width {
				keep[col] = !static[col] && cellVisible(states[i][col], colors)
			}
			p.content.points[i].state = replaceRow(
				p.content.points[i].state,
				cellsToRow(y, states[i], keep, colors),
			)
		}
	}

	slices.SortFunc(p.staticRows, func(a, b ir.Row) int { return a.Y - b.Y })
	p.content = normalizeTimeline(p.duration, p.content.points, rowsEqual)
}

func decodeRowCells(row ir.Row, width int) ([]terminalCell, bool) {
	cells := make([]terminalCell, width)
	occupied := make([]bool, width)
	for i := range cells {
		cells[i].char = ' '
	}
	for _, run := range row.Runs {
		end := runEndCol(run)
		runes := []rune(run.Text)
		if run.StartCol < 0 || end > width || end < run.StartCol || len(runes) != end-run.StartCol {
			return nil, false
		}
		for i, char := range runes {
			// A NUL cell is commonly used as a continuation marker for a wide
			// glyph. Keep such rows on the established whole-row path instead of
			// attempting to split a multi-cell grapheme.
			if char == 0 {
				return nil, false
			}
			col := run.StartCol + i
			if occupied[col] {
				return nil, false
			}
			occupied[col] = true
			cells[col] = terminalCell{char: char, attrs: run.Attrs}
		}
	}
	return cells, true
}

func cellVisible(cell terminalCell, colors *color.Catalog) bool {
	return cell.char != ' ' || cell.attrs.Underline || !colors.IsDefault(cell.attrs.BG)
}

func cellVisualEqual(a, b terminalCell, colors *color.Catalog) bool {
	if !cellVisible(a, colors) && !cellVisible(b, colors) {
		return true
	}
	return a == b
}

func cellsToRow(y int, cells []terminalCell, include []bool, colors *color.Catalog) ir.Row {
	row := ir.Row{Y: y}
	for col := 0; col < len(cells); {
		if !include[col] || !cellVisible(cells[col], colors) {
			col++
			continue
		}
		start := col
		attrs := cells[col].attrs
		text := make([]rune, 0, 8)
		for col < len(cells) && include[col] && cellVisible(cells[col], colors) && cells[col].attrs == attrs {
			text = append(text, cells[col].char)
			col++
		}
		row.Runs = append(row.Runs, ir.TextRun{
			Text:     string(text),
			StartCol: start,
			EndCol:   col,
			Attrs:    attrs,
		})
	}
	return row
}

func replaceRow(rows []ir.Row, replacement ir.Row) []ir.Row {
	out := make([]ir.Row, 0, len(rows)+1)
	inserted := false
	for _, row := range rows {
		if row.Y == replacement.Y {
			if len(replacement.Runs) > 0 {
				out = append(out, replacement)
			}
			inserted = true
			continue
		}
		if !inserted && len(replacement.Runs) > 0 && row.Y > replacement.Y {
			out = append(out, replacement)
			inserted = true
		}
		out = append(out, row)
	}
	if !inserted && len(replacement.Runs) > 0 {
		out = append(out, replacement)
	}
	return out
}
