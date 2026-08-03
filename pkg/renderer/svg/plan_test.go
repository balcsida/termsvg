package svg

import (
	"reflect"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func TestBuildRenderPlanSplitsContentAndCursorTimelines(t *testing.T) {
	row := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "same"}}}
	rec := &ir.Recording{
		Height:   1,
		Duration: 3 * time.Second,
		Frames: []ir.Frame{
			{Time: 0, Rows: []ir.Row{row}, Cursor: ir.Cursor{Visible: true}},
			{Time: time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 1, Visible: true}},
			{Time: time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 2, Visible: true}},
			{Time: 2 * time.Second, Rows: []ir.Row{row}, Cursor: ir.Cursor{Col: 2}},
		},
	}

	plan := buildRenderPlan(rec, true)

	if plan.duration != rec.Duration || len(plan.contentFrames) != 1 {
		t.Fatalf("content plan = duration %v, %d frames", plan.duration, len(plan.contentFrames))
	}
	wantCursor := []cursorPoint{
		{time: 0, cursor: ir.Cursor{Visible: true}},
		{time: time.Second, cursor: ir.Cursor{Col: 2, Visible: true}},
		{time: 2 * time.Second, cursor: ir.Cursor{Col: 2}},
	}
	if !plan.cursor.everVisible || !reflect.DeepEqual(plan.cursor.points, wantCursor) {
		t.Fatalf("cursor plan = %#v, want %#v", plan.cursor, wantCursor)
	}
}

func TestBuildRenderPlanHoistsOnlyRowsStaticAcrossMissingStates(t *testing.T) {
	static := ir.Row{Y: 0, Runs: []ir.TextRun{{Text: "static"}}}
	dynamic := ir.Row{Y: 1, Runs: []ir.TextRun{{Text: "dynamic"}}}
	rec := &ir.Recording{Height: 3, Frames: []ir.Frame{
		{Rows: []ir.Row{static}},
		{Time: time.Second, Rows: []ir.Row{static, dynamic}},
	}}
	wantFrames := append([]ir.Frame(nil), rec.Frames...)
	wantFrames[0].Rows = append([]ir.Row(nil), rec.Frames[0].Rows...)
	wantFrames[1].Rows = append([]ir.Row(nil), rec.Frames[1].Rows...)

	plan := buildRenderPlan(rec, false)

	if !reflect.DeepEqual(plan.staticRows, []ir.Row{static}) {
		t.Fatalf("static rows = %#v, want %#v", plan.staticRows, []ir.Row{static})
	}
	if len(plan.contentFrames) != 2 || len(plan.contentFrames[0].rows) != 0 || !reflect.DeepEqual(plan.contentFrames[1].rows, []ir.Row{dynamic}) {
		t.Fatalf("dynamic frames = %#v", plan.contentFrames)
	}
	if plan.cursor.everVisible || len(plan.cursor.points) != 0 {
		t.Fatalf("disabled cursor plan = %#v", plan.cursor)
	}
	if !reflect.DeepEqual(rec.Frames, wantFrames) {
		t.Fatal("building the render plan mutated source frames")
	}
}
