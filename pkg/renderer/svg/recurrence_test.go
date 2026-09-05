package svg

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestRecurringContentReusesCompleteState(t *testing.T) {
	rec := createTestRecording()
	a := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: strings.Repeat("a", 200)}}}}
	b := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "b"}}}}
	rec.Frames = []ir.Frame{{Rows: a}, {Time: time.Second, Rows: b}, {Time: 2 * time.Second, Rows: a}}
	rec.Duration = 3 * time.Second
	before := cloneRecording(rec)
	c := canvas{
		rec: rec, plan: buildRenderPlan(rec, false), config: *renderer.DefaultConfig(), options: DefaultOptions(),
		classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
	}
	selectors, _ := contentKeyframesFor(c.plan.content)
	content, err := c.prepareContentContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(content.frameRows) != 2 {
		t.Fatalf("stored states = %d; want 2", len(content.frameRows))
	}
	want := []int{0, 1, 0, 0}
	if len(content.frameKeyframes) != len(want) {
		t.Fatalf("keyframes = %+v", content.frameKeyframes)
	}
	for i, frame := range content.frameKeyframes {
		if frame.state != want[i] || frame.selector != selectors[i].selector {
			t.Fatalf("keyframes = %+v", content.frameKeyframes)
		}
	}
	if !reflect.DeepEqual(rec, before) {
		t.Fatal("recording mutated")
	}
}

func TestDistinctContentKeepsOriginalRepresentation(t *testing.T) {
	rec := createTestRecording()
	rec.Frames = []ir.Frame{
		{Rows: []ir.Row{{Runs: []ir.TextRun{{Text: "a"}}}}},
		{Time: time.Second, Rows: []ir.Row{{Runs: []ir.TextRun{{Text: "b"}}}}},
		{Time: 2 * time.Second, Rows: []ir.Row{{Runs: []ir.TextRun{{Text: "c"}}}}},
	}
	rec.Duration = 3 * time.Second
	c := canvas{
		rec: rec, plan: buildRenderPlan(rec, false), config: *renderer.DefaultConfig(), options: DefaultOptions(),
		classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
	}
	baseline, err := c.prepareContentRepresentation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.prepareContentContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, baseline) {
		t.Fatal("distinct timeline changed representation")
	}
}

func TestRecurringContentCostGateKeepsBaseline(t *testing.T) {
	baseline := preparedContent{frameStateIDs: []string{"baseline"}, cost: preparedContentCost{regionBytes: 100}}
	for _, cost := range []int64{100, 101} {
		reused := preparedContent{frameStateIDs: []string{"reused"}, cost: preparedContentCost{regionBytes: cost}}
		if got := strictSmallestPrepared(baseline, reused); !reflect.DeepEqual(got, baseline) {
			t.Fatalf("equal or larger reused cost %d replaced baseline", cost)
		}
	}
}

func TestRecurringStateIndicesResolveCollisions(t *testing.T) {
	a := []ir.Row{
		{Y: 1, Runs: []ir.TextRun{{Text: "same", StartCol: 2, EndCol: 6}}},
		{Y: 2, Runs: []ir.TextRun{{Text: "tail"}}},
	}
	states := make([][]ir.Row, 1, 8)
	states[0] = a
	for _, change := range []func([]ir.Row){
		func(rows []ir.Row) { rows[0].Y++ },
		func(rows []ir.Row) { rows[0].Runs[0].StartCol++ },
		func(rows []ir.Row) { rows[0].Runs[0].EndCol++ },
		func(rows []ir.Row) { rows[0].Runs[0].Text = "other" },
		func(rows []ir.Row) { rows[0].Runs[0].Attrs.FG++ },
		func(rows []ir.Row) { rows[0], rows[1] = rows[1], rows[0] },
	} {
		rows := slices.Clone(a)
		rows[0].Runs = slices.Clone(rows[0].Runs)
		change(rows)
		states = append(states, rows)
	}
	states = append(states, a)
	for range 3 {
		mapping, unique := recurringStateIndices(states, func([]ir.Row) uint64 { return 0 })
		if !reflect.DeepEqual(mapping, []int{0, 1, 2, 3, 4, 5, 6, 0}) || len(unique) != 7 {
			t.Fatalf("mapping=%v unique=%d", mapping, len(unique))
		}
	}
	if a[0].Y != 1 || a[0].Runs[0].StartCol != 2 || a[0].Runs[0].Text != "same" {
		t.Fatal("input mutated")
	}
}

func TestRecurringContentKeepsCheapestCompleteRepresentation(t *testing.T) {
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll} {
		for _, mode := range []FrameSwitchMode{FrameSwitchTranslate, FrameSwitchHref} {
			for _, minify := range []bool{false, true} {
				rec := createTestRecording()
				a := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: strings.Repeat("shared", 30)}}}}
				b := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "b"}}}}
				rec.Frames = []ir.Frame{{Rows: a}, {Time: time.Second, Rows: b}, {Time: 2 * time.Second, Rows: a}}
				rec.Duration = 3 * time.Second
				config := *renderer.DefaultConfig()
				config.Minify = minify
				options := DefaultOptions()
				options.Layout = layout
				options.FrameSwitch = mode
				if mode == FrameSwitchHref {
					options.Animation = AnimationSMIL
				}
				c := canvas{
					rec: rec, plan: buildRenderPlan(rec, false), config: config, options: options,
					classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{},
				}
				baseline, err := c.prepareContentRepresentation(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				probe := c
				probe.reuseContentStates = true
				reused, err := probe.prepareContentRepresentation(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				got, err := c.prepareContentContext(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				want := baseline
				if reused.cost.regionBytes < baseline.cost.regionBytes {
					want = reused
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("layout=%s switch=%s minify=%v: selected cost=%d baseline=%d reused=%d",
						layout, mode, minify, got.cost.regionBytes, baseline.cost.regionBytes, reused.cost.regionBytes)
				}
				assertRecurringPartitionPreserved(t, &baseline, &got)
			}
		}
	}
}

func assertRecurringPartitionPreserved(t *testing.T, baseline, got *preparedContent) {
	t.Helper()
	if len(got.bands) != len(baseline.bands) {
		t.Fatal("recurrence changed the chosen partition")
	}
	for i := range got.bands {
		band, original := &got.bands[i], &baseline.bands[i]
		if band.x != original.x || band.y != original.y || band.width != original.width ||
			band.height != original.height || band.kind != original.kind {
			t.Fatal("recurrence changed viewport geometry or primitive representation")
		}
	}
}
