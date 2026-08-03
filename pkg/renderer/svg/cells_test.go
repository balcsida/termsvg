package svg

import (
	"image/color"
	"slices"
	"testing"
	"time"

	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
)

func testCatalog() *termcolor.Catalog {
	return termcolor.NewCatalog(color.RGBA{R: 255, G: 255, B: 255, A: 255}, color.RGBA{A: 255})
}

func TestHoistStaticCellsExtractsStablePrefix(t *testing.T) {
	colors := testCatalog()
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "CPU: 1", StartCol: 0, EndCol: 6}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "CPU: 2", StartCol: 0, EndCol: 6}}}}},
		}},
	}

	plan.hoistStaticCells(6, 1, colors)

	if len(plan.staticRows) != 1 || len(plan.staticRows[0].Runs) != 1 || plan.staticRows[0].Runs[0].Text != "CPU:" {
		t.Fatalf("static rows = %#v; want stable CPU prefix", plan.staticRows)
	}
	for i, point := range plan.content.points {
		if len(point.state) != 1 || len(point.state[0].Runs) != 1 {
			t.Fatalf("state %d = %#v; want one dynamic run", i, point.state)
		}
		run := point.state[0].Runs[0]
		if run.StartCol != 5 || run.EndCol != 6 {
			t.Fatalf("state %d dynamic extent = [%d,%d); want [5,6)", i, run.StartCol, run.EndCol)
		}
	}
}

func TestHoistStaticCellsPreservesUnderlinedSpaces(t *testing.T) {
	colors := testCatalog()
	attrs := ir.CellAttrs{Underline: true}
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: " 1", StartCol: 0, EndCol: 2, Attrs: attrs}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: " 2", StartCol: 0, EndCol: 2, Attrs: attrs}}}}},
		}},
	}

	plan.hoistStaticCells(2, 1, colors)

	if len(plan.staticRows) != 1 || plan.staticRows[0].Runs[0].Text != " " || !plan.staticRows[0].Runs[0].Attrs.Underline {
		t.Fatalf("underlined static space was not preserved: %#v", plan.staticRows)
	}
}

func TestHoistStaticCellsSkipsAmbiguousCellExtents(t *testing.T) {
	colors := testCatalog()
	original := []timelinePoint[[]ir.Row]{
		{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "界", StartCol: 0, EndCol: 2}}}}},
		{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "界", StartCol: 0, EndCol: 2}}}}},
	}
	plan := renderPlan{duration: time.Second, content: timeline[[]ir.Row]{duration: time.Second, points: original}}

	plan.hoistStaticCells(2, 1, colors)

	if len(plan.staticRows) != 0 || !rowsEqual(plan.content.points[0].state, original[0].state) {
		t.Fatalf("ambiguous wide-cell run was modified: static=%#v content=%#v", plan.staticRows, plan.content.points)
	}
}

func TestHoistStaticCellsSkipsNULContinuationCells(t *testing.T) {
	colors := testCatalog()
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: string([]rune{'界', 0}), StartCol: 0, EndCol: 2}}}
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{row}},
			{time: time.Second, state: []ir.Row{row}},
		}},
	}

	plan.hoistStaticCells(2, 1, colors)

	if len(plan.staticRows) != 0 {
		t.Fatalf("wide-glyph continuation was split into static cells: %#v", plan.staticRows)
	}
}

func TestHoistStaticCellsExtractsColoredBlankSpan(t *testing.T) {
	colors := testCatalog()
	bg := termcolor.ID(1)
	attrs := ir.CellAttrs{BG: bg}
	plan := renderPlan{
		duration: time.Second,
		content: timeline[[]ir.Row]{duration: time.Second, points: []timelinePoint[[]ir.Row]{
			{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: " A", StartCol: 0, EndCol: 2, Attrs: attrs}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: " B", StartCol: 0, EndCol: 2, Attrs: attrs}}}}},
		}},
	}

	plan.hoistStaticCells(2, 1, colors)

	if len(plan.staticRows) != 1 || plan.staticRows[0].Runs[0].Text != " " || plan.staticRows[0].Runs[0].Attrs.BG != bg {
		t.Fatalf("colored static blank was not extracted: %#v", plan.staticRows)
	}
}

func TestHoistStaticCellsPreservesEveryVisualState(t *testing.T) {
	colors := testCatalog()
	original := []timelinePoint[[]ir.Row]{
		{time: 0, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
			{Text: "left", StartCol: 0, EndCol: 4},
			{Text: "1", StartCol: 8, EndCol: 9},
		}}}},
		{time: time.Second / 2, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
			{Text: "left", StartCol: 0, EndCol: 4},
			{Text: "2", StartCol: 8, EndCol: 9},
		}}}},
		{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
			{Text: "left", StartCol: 0, EndCol: 4},
			{Text: "3", StartCol: 8, EndCol: 9},
		}}}},
	}
	plan := renderPlan{
		duration: time.Second,
		content:  timeline[[]ir.Row]{duration: time.Second, points: append([]timelinePoint[[]ir.Row](nil), original...)},
	}

	plan.hoistStaticCells(12, 1, colors)

	for i, point := range plan.content.points {
		want, ok := decodeRowCells(rowAt(original[i].state, 0), 12)
		if !ok {
			t.Fatal("test fixture did not decode")
		}
		got := make([]terminalCell, 12)
		for col := range got {
			got[col].char = ' '
		}
		for _, rows := range [][]ir.Row{plan.staticRows, point.state} {
			part, ok := decodeRowCells(rowAt(rows, 0), 12)
			if !ok {
				t.Fatalf("optimized state %d did not decode", i)
			}
			for col, cell := range part {
				if cellVisible(cell, colors) {
					got[col] = cell
				}
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("state %d visual cells changed:\n got  %#v\n want %#v", i, got, want)
		}
	}
}
