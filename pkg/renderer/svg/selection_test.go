package svg

import (
	"bytes"
	"context"
	"log"
	"reflect"
	"strings"
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestAutomaticSwitchValidation(t *testing.T) {
	options := DefaultOptions()
	options.FrameSwitch = FrameSwitchAuto
	if err := options.Validate(); err == nil || !strings.Contains(err.Error(), "requires SMIL") {
		t.Fatalf("CSS auto switching: %v", err)
	}
	options.Animation = AnimationSMIL
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	options.AutoObjective = AutoObjectiveRuntime
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	options.Primitives = PrimitiveRectTracks
	if err := options.Validate(); err == nil {
		t.Fatal("rectangle tracks accepted frames")
	}
	options.Layout = LayoutAuto
	if err := options.Validate(); err == nil {
		t.Fatal("rectangle tracks accepted auto layout")
	}
}

//nolint:funlen,gocognit // Keep all option dimensions visible in this exhaustive selection matrix.
func TestExpandedSelectionMatchesConcreteCandidates(t *testing.T) {
	ctx := context.Background()
	rec := scrollRecording(30, 8, 10)
	before := cloneRecording(rec)
	for _, loops := range []int{-1, 0, 2} {
		for _, animation := range []AnimationMode{AnimationCSS, AnimationSMIL} {
			switches := []FrameSwitchMode{FrameSwitchTranslate}
			if animation == AnimationSMIL {
				switches = append(switches, FrameSwitchHref, FrameSwitchAuto)
			}
			for _, switching := range switches {
				for _, layout := range []LayoutMode{LayoutAuto, LayoutBands, LayoutScroll} {
					for _, objective := range []AutoObjective{AutoObjectiveSize, AutoObjectiveRuntime} {
						if layout != LayoutAuto && switching != FrameSwitchAuto && objective == AutoObjectiveRuntime {
							continue
						}
						config := renderer.DefaultConfig()
						config.LoopCount, config.Minify = loops, true
						r := New(config, WithLayout(layout), WithAnimation(animation), WithFrameSwitch(switching),
							WithAutoObjective(objective), WithStyleMode(StyleAuto), WithMaxFPS(3))
						plan, err := r.buildSemanticPlan(ctx, rec)
						if err != nil {
							t.Fatal(err)
						}
						originalPlan := cloneSemanticPlan(&plan)
						layouts := []LayoutMode{layout}
						if layout == LayoutAuto {
							layouts = []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions}
							if animation == AnimationSMIL || infiniteLoop(loops) {
								layouts = append(layouts, LayoutScroll)
							}
						}
						modes := []FrameSwitchMode{switching}
						if switching == FrameSwitchAuto {
							modes = []FrameSwitchMode{FrameSwitchTranslate, FrameSwitchHref}
						}
						candidates := make([]*preparedCandidate, 0, len(layouts)*len(modes))
						for _, candidateLayout := range layouts {
							for _, mode := range modes {
								options := r.options
								options.Layout, options.FrameSwitch = candidateLayout, mode
								candidate, err := prepareCandidate(ctx, rec, &plan, config, options)
								if err != nil {
									t.Fatal(err)
								}
								if err := r.measureCandidate(ctx, rec, candidate); err != nil {
									t.Fatal(err)
								}
								candidates = append(candidates, candidate)
							}
						}
						want := selectPreparedCandidate(objective, candidates...)
						selected, err := r.prepareSelectedCandidate(ctx, rec, &plan)
						if err != nil {
							t.Fatal(err)
						}
						if selected.options != want.options {
							t.Fatalf("loops=%d animation=%s layout=%s switching=%s objective=%s: selected %+v; want %+v",
								loops, animation, layout, switching, objective, selected.options, want.options)
						}
						if selected.plan != &plan || !reflect.DeepEqual(plan, originalPlan) {
							t.Fatal("selection did not retain immutable shared plan")
						}
						var expected bytes.Buffer
						if err := r.serializeCandidate(ctx, rec, &expected, want); err != nil {
							t.Fatal(err)
						}
						if int64(expected.Len()) != want.metrics.FinalBytes {
							t.Fatalf("measured %d bytes; serialized %d", want.metrics.FinalBytes, expected.Len())
						}
						builds, serializations := 0, 0
						r.onSemanticPlanBuild = func() { builds++ }
						r.onCandidateSerialize = func() { serializations++ }
						for range 2 {
							var output bytes.Buffer
							if err := r.Render(ctx, rec, &output); err != nil {
								t.Fatal(err)
							}
							if !bytes.Equal(output.Bytes(), expected.Bytes()) {
								t.Fatal("automatic output differs from concrete winner")
							}
						}
						if builds != 2 || serializations != 2 {
							t.Fatalf("builds=%d serializations=%d", builds, serializations)
						}
					}
				}
			}
		}
	}
	if !reflect.DeepEqual(rec, before) {
		t.Fatal("selection mutated recording")
	}
}

func TestExpandedSelectionKeepsEstablishedOrderOnTies(t *testing.T) {
	candidates := make([]*preparedCandidate, 0, 8)
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands, LayoutRegions, LayoutScroll} {
		for _, mode := range []FrameSwitchMode{FrameSwitchTranslate, FrameSwitchHref} {
			candidates = append(candidates, &preparedCandidate{
				options: Options{Layout: layout, FrameSwitch: mode}, metrics: CandidateMetrics{FinalBytes: 10},
			})
		}
	}
	for _, objective := range []AutoObjective{AutoObjectiveSize, AutoObjectiveRuntime} {
		if got := selectPreparedCandidate(objective, candidates...); got != candidates[0] {
			t.Fatalf("tie selected %+v", got.options)
		}
		if got := selectPreparedCandidate(objective, candidates[2:]...); got != candidates[2] {
			t.Fatalf("tie selected %+v", got.options)
		}
	}
}

//nolint:gocognit // Assert the complete candidate table for each animation and loop configuration.
func TestAutoScrollEligibilityAndSwitchDebug(t *testing.T) {
	oldWriter, oldFlags := log.Writer(), log.Flags()
	defer func() { log.SetOutput(oldWriter); log.SetFlags(oldFlags) }()
	log.SetFlags(0)
	for _, animation := range []AnimationMode{AnimationCSS, AnimationSMIL} {
		for _, loops := range []int{-1, 0, 2} {
			config := renderer.DefaultConfig()
			config.Debug, config.LoopCount = true, loops
			switching := FrameSwitchTranslate
			if animation == AnimationSMIL {
				switching = FrameSwitchAuto
			}
			r := New(config, WithLayout(LayoutAuto), WithAnimation(animation), WithFrameSwitch(switching))
			var output, diagnostics bytes.Buffer
			log.SetOutput(&diagnostics)
			if err := r.Render(context.Background(), scrollRecording(30, 8, 10), &output); err != nil {
				t.Fatal(err)
			}
			wantScroll := animation == AnimationSMIL || infiniteLoop(loops)
			if got := strings.Contains(diagnostics.String(), "candidate layout=scroll"); got != wantScroll {
				t.Fatalf("%s loops=%d scroll=%t; want %t", animation, loops, got, wantScroll)
			}
			wantCandidates := 3
			if wantScroll {
				wantCandidates++
			}
			if switching == FrameSwitchAuto {
				wantCandidates *= 2
			}
			if got := strings.Count(diagnostics.String(), "auto candidate"); got != wantCandidates {
				t.Fatalf("candidate count %d; want %d", got, wantCandidates)
			}
			for _, field := range []string{"switching=translate", "objective=size", "selected=", "selected_switching="} {
				if !strings.Contains(diagnostics.String(), field) {
					t.Fatalf("missing debug field %s", field)
				}
			}
			if switching == FrameSwitchAuto && !strings.Contains(diagnostics.String(), "switching=href") {
				t.Fatal("missing href candidate")
			}
		}
	}
}

func TestFiniteSMILScrollPreservesStatesAndFrozenEndpoint(t *testing.T) {
	rec := scrollRecording(30, 8, 10)
	for _, loops := range []int{-1, 2} {
		config := renderer.DefaultConfig()
		config.LoopCount = loops
		for _, layout := range []LayoutMode{LayoutFrames, LayoutScroll} {
			for _, mode := range []FrameSwitchMode{FrameSwitchTranslate, FrameSwitchHref} {
				assertSemanticParityWithConfig(t, rec, config,
					WithLayout(layout), WithAnimation(AnimationSMIL), WithFrameSwitch(mode))
				var out bytes.Buffer
				r := New(config, WithLayout(layout), WithAnimation(AnimationSMIL), WithFrameSwitch(mode))
				if err := r.Render(context.Background(), rec, &out); err != nil {
					t.Fatal(err)
				}
				animations := strings.Count(out.String(), " calcMode=\"discrete\"")
				if animations == 0 || animations != strings.Count(out.String(), " fill=\"freeze\"") {
					t.Fatalf("%s/%s does not freeze every finite animation", layout, mode)
				}
			}
		}
	}
}
