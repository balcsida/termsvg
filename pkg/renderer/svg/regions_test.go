package svg

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestBuildDynamicRegionsKeepsDistantIntervalsSeparate(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 12, height: 1, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "0", StartCol: 1, EndCol: 2}, {Text: "0", StartCol: 9, EndCol: 10},
			}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{
				{Text: "1", StartCol: 1, EndCol: 2}, {Text: "1", StartCol: 9, EndCol: 10},
			}}}},
		},
	}}

	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 2 {
		t.Fatalf("regions = %#v; want two distant regions", regions)
	}
	got := [][4]int{
		{regions[0].x, regions[0].y, regions[0].width, regions[0].height},
		{regions[1].x, regions[1].y, regions[1].width, regions[1].height},
	}
	if !reflect.DeepEqual(got, [][4]int{{1, 0, 1, 1}, {9, 0, 1, 1}}) {
		t.Fatalf("region bounds = %#v", got)
	}
}

func TestOptimizeDynamicRegionsUsesExactBackendProfitability(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 8, height: 2, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "..", StartCol: 1, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "..", StartCol: 3, EndCol: 5}}},
			}},
			{time: time.Second, state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "##", StartCol: 1, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "##", StartCol: 3, EndCol: 5}}},
			}},
		},
	}}

	for _, options := range []Options{
		{Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate},
		{Layout: LayoutRegions, Animation: AnimationSMIL, FrameSwitch: FrameSwitchHref},
	} {
		c := canvas{
			rec: parityRecording(8, 2, nil), plan: plan, config: *renderer.DefaultConfig(),
			options: options, classNames: testCatalog().GenerateClassNames(), metrics: &CandidateMetrics{},
		}
		c.rec.Colors = testCatalog()
		regions := buildDynamicRegions(&plan, c.rec.Colors)
		if len(regions) != 2 {
			t.Fatalf("initial regions = %#v; want two smallest intervals", regions)
		}
		separate, err := c.serializedRegionBytes(context.Background(), regions)
		if err != nil {
			t.Fatal(err)
		}
		merged := mergeDynamicRegions(&plan, regions, 0, 1, c.rec.Colors)
		mergedBytes, err := c.serializedRegionBytes(context.Background(), merged)
		if err != nil {
			t.Fatal(err)
		}
		if mergedBytes >= separate {
			t.Fatalf("%+v fixture is not profitable: merged %d, separate %d", options, mergedBytes, separate)
		}
		optimized, err := c.optimizeDynamicRegions(context.Background(), regions)
		if err != nil {
			t.Fatal(err)
		}
		if len(optimized) != 1 || optimized[0].x != 1 || optimized[0].y != 0 ||
			optimized[0].width != 4 || optimized[0].height != 2 {
			t.Fatalf("%+v optimized regions = %#v", options, optimized)
		}
	}
}

func TestRegionCostRepresentationExcludesUnrelatedSVG(t *testing.T) {
	rec := experimentalRecording()
	config := renderer.DefaultConfig()
	plan, err := buildSemanticPlan(context.Background(), rec, true, 0, config.LoopCount)
	if err != nil {
		t.Fatal(err)
	}
	c := canvas{rec: rec, plan: plan, config: *config, options: Options{
		Layout: LayoutRegions, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate,
	}, classNames: rec.Colors.GenerateClassNames(), metrics: &CandidateMetrics{}}
	content, err := c.prepareLocalViewports(context.Background(), c.regionBands(buildDynamicRegions(&plan, rec.Colors)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := c.renderRegionRepresentation(context.Background(), &out, &content); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"<defs>", "<svg x=", "@keyframes"} {
		if !strings.Contains(out.String(), required) {
			t.Fatalf("region representation missing %q: %s", required, out.String())
		}
	}
	for _, unrelated := range []string{"clipPath", "static", "cursor", "<circle"} {
		if strings.Contains(out.String(), unrelated) {
			t.Fatalf("region representation contains unrelated %q: %s", unrelated, out.String())
		}
	}
}

func TestOptimizeRegionMergesKeepsTiesSeparate(t *testing.T) {
	regions := []dynamicRegion{{x: 1, width: 1, height: 1}, {x: 2, y: 1, width: 1, height: 1}}
	got, err := optimizeRegionMerges(regions, func([]dynamicRegion) (int64, error) { return 100, nil },
		mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("tie merged regions: got %#v, want %#v", got, regions)
	}
}

func TestOptimizeRegionMergeCostUsesCompleteRegionRepresentation(t *testing.T) {
	regions := []dynamicRegion{
		{x: 1, width: 1, height: 1},
		{x: 1, y: 1, width: 1, height: 1},
		{x: 5, y: 5, width: 1, height: 1},
	}
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		switch len(candidate) {
		case 3:
			return 100, nil
		case 2:
			if candidate[0].height == 2 || candidate[1].height == 2 {
				return 101, nil
			}
			return 200, nil
		default:
			return 50, nil
		}
	}, mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("pair-isolated savings merged a larger complete representation: %#v", got)
	}
}

func TestOptimizeRegionMergesMeasuresUnchangedSetOnce(t *testing.T) {
	regions := []dynamicRegion{
		{x: 1, width: 1, height: 1},
		{x: 1, y: 1, width: 1, height: 1},
		{x: 1, y: 2, width: 1, height: 1},
		{x: 1, y: 3, width: 1, height: 1},
	}
	initialMeasurements := 0
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		if reflect.DeepEqual(candidate, regions) {
			initialMeasurements++
		}
		return 100, nil
	}, mergeRegionBounds)
	if err != nil {
		t.Fatal(err)
	}
	if initialMeasurements != 1 {
		t.Fatalf("unchanged region set measurements = %d, want 1", initialMeasurements)
	}
	if !reflect.DeepEqual(got, regions) {
		t.Fatalf("tie changed final regions: got %#v, want %#v", got, regions)
	}
}

func mergeRegionBounds(regions []dynamicRegion, i, j int) []dynamicRegion {
	a, b := regions[i], regions[j]
	merged := dynamicRegion{x: min(a.x, b.x), y: min(a.y, b.y)}
	merged.width = max(a.x+a.width, b.x+b.width) - merged.x
	merged.height = max(a.y+a.height, b.y+b.height) - merged.y
	out := make([]dynamicRegion, 0, len(regions)-1)
	for index := range regions {
		if index != i && index != j {
			out = append(out, regions[index])
		}
	}
	return append(out, merged)
}

func TestOptimizeRegionMergesRejectsBoundingBoxThroughThirdRegion(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 3, height: 2, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "a", EndCol: 1}, {Text: "c", StartCol: 2, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "bbb", EndCol: 3}}},
			}},
			{time: time.Second, state: []ir.Row{
				{Y: 0, Runs: []ir.TextRun{{Text: "A", EndCol: 1}, {Text: "C", StartCol: 2, EndCol: 3}}},
				{Y: 1, Runs: []ir.TextRun{{Text: "BBB", EndCol: 3}}},
			}},
		},
	}}
	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 3 {
		t.Fatalf("initial regions = %#v", regions)
	}
	got, err := optimizeRegionMerges(regions, func(candidate []dynamicRegion) (int64, error) {
		return int64(len(candidate) * 100), nil
	}, func(candidate []dynamicRegion, i, j int) []dynamicRegion {
		return mergeDynamicRegions(&plan, candidate, i, j, testCatalog())
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("unsafe merge retained %d regions: %#v", len(got), got)
	}
}

func TestRegionRenderDoesNotDuplicatePaintInsideMergedBounds(t *testing.T) {
	rec := parityRecording(3, 2, [][]ir.Row{
		{
			parityRow(0, parityRun("a", 0, ir.CellAttrs{}), parityRun("c", 2, ir.CellAttrs{})),
			parityRow(1, parityRun("bbb", 0, ir.CellAttrs{})),
		},
		{
			parityRow(0, parityRun("A", 0, ir.CellAttrs{}), parityRun("C", 2, ir.CellAttrs{})),
			parityRow(1, parityRun("BBB", 0, ir.CellAttrs{})),
		},
	})
	config := renderer.DefaultConfig()
	config.ShowCursor = false
	var out bytes.Buffer
	if err := New(config, WithLayout(LayoutRegions)).Render(context.Background(), rec, &out); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(out.Bytes(), []byte("<svg")); got != 4 {
		t.Fatalf("SVG elements = %d, want root plus three safe regions: %s", got, out.String())
	}
}

func TestBuildDynamicRegionsFallsBackForUnsplittableWideGlyph(t *testing.T) {
	plan := renderPlan{duration: time.Second, width: 4, height: 1, content: timeline[[]ir.Row]{
		duration: time.Second,
		points: []timelinePoint[[]ir.Row]{
			{state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "ab", StartCol: 0, EndCol: 3}}}}},
			{time: time.Second, state: []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "cd", StartCol: 0, EndCol: 3}}}}},
		},
	}}

	regions := buildDynamicRegions(&plan, testCatalog())
	if len(regions) != 1 || len(regions[0].fallbackRows) != 2 || regions[0].width != 4 {
		t.Fatalf("fallback regions = %#v", regions)
	}
}
