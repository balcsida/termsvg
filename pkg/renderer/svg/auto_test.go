package svg

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestLayoutAutoValidatesWithoutChangingDefault(t *testing.T) {
	if DefaultOptions().Layout != LayoutFrames {
		t.Fatalf("default layout = %q; want frames", DefaultOptions().Layout)
	}
	options := DefaultOptions()
	options.Layout = LayoutAuto
	if err := options.Validate(); err != nil {
		t.Fatalf("auto layout failed validation: %v", err)
	}
}

func TestCountingWriterCountsAllBytes(t *testing.T) {
	counter := &countingWriter{}
	for _, value := range []string{"abc", "", "defgh"} {
		if _, err := counter.Write([]byte(value)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if counter.size() != 8 {
		t.Fatalf("counted bytes = %d; want 8", counter.size())
	}
}

func TestCountingWriterUsesPostMinifyNBSPWidth(t *testing.T) {
	counter := &countingWriter{collapseNBSP: true}
	if _, err := counter.Write([]byte{'a', 0xc2}); err != nil {
		t.Fatal(err)
	}
	if _, err := counter.Write([]byte{0xa0, 'b'}); err != nil {
		t.Fatal(err)
	}
	if counter.size() != int64(len("a b")) {
		t.Fatalf("counted transformed bytes = %d; want %d", counter.size(), len("a b"))
	}
}

func TestPreparedCandidatesShareOneImmutableSemanticPlan(t *testing.T) {
	rec := experimentalRecording()
	plan, err := buildSemanticPlan(context.Background(), rec, true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneSemanticPlan(&plan)
	config := renderer.DefaultConfig()

	frames, err := prepareCandidate(context.Background(), rec, &plan, config, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	bandOptions := DefaultOptions()
	bandOptions.Layout = LayoutBands
	bands, err := prepareCandidate(context.Background(), rec, &plan, config, bandOptions)
	if err != nil {
		t.Fatal(err)
	}

	if frames.plan != &plan || bands.plan != &plan {
		t.Fatal("prepared candidates do not share the semantic plan")
	}
	if !reflect.DeepEqual(plan, before) {
		t.Fatal("candidate preparation mutated the semantic plan")
	}
}

func TestSelectPreparedCandidateKeepsFramesOnTie(t *testing.T) {
	frames := preparedCandidate{options: Options{Layout: LayoutFrames}, metrics: CandidateMetrics{FinalBytes: 10}}
	bands := preparedCandidate{options: Options{Layout: LayoutBands}, metrics: CandidateMetrics{FinalBytes: 10}}
	regions := preparedCandidate{options: Options{Layout: LayoutRegions}, metrics: CandidateMetrics{FinalBytes: 10}}
	if got := selectPreparedCandidate(&frames, &bands, &regions); got != &frames {
		t.Fatalf("tie selected %q; want frames", got.options.Layout)
	}
	bands.metrics.FinalBytes = 9
	regions.metrics.FinalBytes = 9
	if got := selectPreparedCandidate(&frames, &bands, &regions); got != &bands {
		t.Fatalf("band/region tie selected %q; want bands", got.options.Layout)
	}
}

func TestSemanticPlanningHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildSemanticPlan(ctx, experimentalRecording(), true, 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("buildSemanticPlan() error = %v; want context canceled", err)
	}
}

func TestAutoSerializesSmallestPreparedCandidate(t *testing.T) {
	rec := experimentalRecording()
	config := renderer.DefaultConfig()
	config.Minify = true
	outputs := map[LayoutMode][]byte{}
	metrics := map[LayoutMode]CandidateMetrics{}
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions} {
		r := New(config, WithLayout(layout))
		var out bytes.Buffer
		if err := r.Render(context.Background(), rec, &out); err != nil {
			t.Fatal(err)
		}
		var final bytes.Buffer
		if err := svgoutput.Write(&final, func(w io.Writer) error {
			_, err := w.Write(out.Bytes())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		outputs[layout] = out.Bytes()
		measured, err := r.MeasureCandidate(context.Background(), rec)
		if err != nil {
			t.Fatal(err)
		}
		metrics[layout] = measured
		if want := int64(final.Len()); measured.FinalBytes != want {
			t.Fatalf("%s final bytes = %d; want %d", layout, measured.FinalBytes, want)
		}
	}
	want := LayoutFrames
	if metrics[LayoutBands].FinalBytes < metrics[LayoutFrames].FinalBytes {
		want = LayoutBands
	}
	if metrics[LayoutRegions].FinalBytes < metrics[want].FinalBytes {
		want = LayoutRegions
	}
	var got bytes.Buffer
	if err := New(config, WithLayout(LayoutAuto)).Render(context.Background(), rec, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), outputs[want]) {
		t.Fatalf("auto output differs from selected %s candidate", want)
	}
}

func TestAutoRenderBuildsSemanticPlanOnce(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithLayout(LayoutAuto))
	var builds atomic.Int64
	r.onSemanticPlanBuild = func() { builds.Add(1) }

	if err := r.Render(context.Background(), experimentalRecording(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("semantic plan builds = %d; want 1", got)
	}
}

func TestAutoRenderSerializesOnlySelectedCandidate(t *testing.T) {
	r := New(renderer.DefaultConfig(), WithLayout(LayoutAuto))
	plans, serializations := 0, 0
	r.onSemanticPlanBuild = func() { plans++ }
	r.onCandidateSerialize = func() { serializations++ }
	if err := r.Render(context.Background(), experimentalRecording(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if plans != 1 || serializations != 1 {
		t.Fatalf("semantic plans/serializations = %d/%d; want 1/1", plans, serializations)
	}
}

func cloneSemanticPlan(source *semanticPlan) semanticPlan {
	plan := *source
	cloneRows := func(rows []ir.Row) []ir.Row {
		out := slices.Clone(rows)
		for i := range out {
			out[i].Runs = slices.Clone(out[i].Runs)
		}
		return out
	}
	plan.staticRows = cloneRows(plan.staticRows)
	plan.content.points = slices.Clone(plan.content.points)
	for i := range plan.content.points {
		plan.content.points[i].state = cloneRows(plan.content.points[i].state)
	}
	plan.cursor.points = slices.Clone(plan.cursor.points)
	plan.usedColors = slices.Clone(plan.usedColors)
	return plan
}
