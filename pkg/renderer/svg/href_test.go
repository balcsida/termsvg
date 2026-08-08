package svg

import (
	"strings"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestHrefFrameSwitchRequiresSMIL(t *testing.T) {
	options := DefaultOptions()
	options.FrameSwitch = FrameSwitchHref
	if err := options.Validate(); err == nil {
		t.Fatal("href switching with CSS unexpectedly validated")
	}
	options.Animation = AnimationSMIL
	if err := options.Validate(); err != nil {
		t.Fatalf("href switching with SMIL failed validation: %v", err)
	}
}

func TestWriteSMILHrefUsesOneDiscreteReferenceTimeline(t *testing.T) {
	var out strings.Builder
	canvas := canvas{plan: renderPlan{duration: time.Second}}
	frames := []keyframePoint[int]{
		{selector: "0%", state: 0},
		{selector: "50%", state: 1},
		{selector: "100%", state: 0},
	}

	canvas.writeSMILHref(&out, frames, []string{"_f0", "_f1"})

	got := out.String()
	for _, want := range []string{
		`<animate attributeName="href"`,
		`values="#_f0;#_f1;#_f0"`,
		`keyTimes="0;.5;1"`,
		`calcMode="discrete"`,
		`repeatCount="indefinite"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SMIL href output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "animateTransform") {
		t.Fatalf("href switching emitted a transform animation: %q", got)
	}
}

func TestWriteHrefSequenceUsesSingleRuntimeUse(t *testing.T) {
	var out strings.Builder
	canvas := canvas{w: &out, plan: renderPlan{duration: time.Second}}
	frames := []keyframePoint[int]{{selector: "0%", state: 1}, {selector: "100%", state: 0}}

	canvas.writeHrefSequence(frames, []string{"_f0", "_f1"})

	got := out.String()
	if strings.Count(got, "<use ") != 1 || !strings.HasPrefix(got, `<use href="#_f1">`) {
		t.Fatalf("href sequence = %q; want one use starting at _f1", got)
	}
}

func TestWriteStateDefinitionElidesOnlyOneGeneratedChild(t *testing.T) {
	tests := []struct {
		name   string
		minify bool
		rows   []*renderedRow
		want   string
	}{
		{name: "empty", rows: nil, want: `<g id="_f0"></g>`},
		{name: "empty minified", minify: true, rows: nil, want: `<g id="_f0"/>`},
		{name: "text", rows: []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text y="20">x</text>`}}, want: `<text id="_f0" y="20">x</text>`},
		{name: "rect", rows: []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<rect y="0" width="12"/>`}}, want: `<rect id="_f0" y="0" width="12"/>`},
		{name: "multiple", rows: []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text>x</text>`}, {row: ir.Row{Runs: []ir.TextRun{{Text: "y"}}}, svg: `<text>y</text>`}}, want: `<g id="_f0"><text>x</text><text>y</text></g>`},
		{name: "row use", rows: []*renderedRow{{id: "a"}}, want: `<use id="_f0" href="#a"/>`},
		{name: "existing child id", rows: []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text id="row">x</text>`}}, want: `<g id="_f0"><text id="row">x</text></g>`},
		{name: "existing child id minified", minify: true, rows: []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text id="row">x</text>`}}, want: `<g id="_f0"><text id="row">x</text></g>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			canvas := canvas{w: &out, rec: createTestRecording(), config: renderer.Config{Minify: tt.minify}}
			canvas.writeStateDefinition("_f0", tt.rows)
			if got := out.String(); got != tt.want {
				t.Fatalf("state definition = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestSingleChildStateElisionSavesExactBytes(t *testing.T) {
	const before = `<g id="_f0"><text y="20">x</text></g>`
	var out strings.Builder
	canvas := canvas{w: &out, rec: createTestRecording()}
	canvas.writeStateDefinition("_f0", []*renderedRow{{row: ir.Row{Runs: []ir.TextRun{{Text: "x"}}}, svg: `<text y="20">x</text>`}})
	if delta := len(before) - out.Len(); delta != 7 {
		t.Fatalf("single-child state byte delta = %d; want 7", delta)
	}
}

func TestStateIDsUseReservedPrefix(t *testing.T) {
	got := stateIDs("_b2_", 3)
	want := []string{"_b2_0", "_b2_1", "_b2_2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stateIDs()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
