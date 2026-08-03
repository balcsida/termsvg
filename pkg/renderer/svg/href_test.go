package svg

import (
	"strings"
	"testing"
	"time"
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

func TestStateIDsUseReservedPrefix(t *testing.T) {
	got := stateIDs("_b2_", 3)
	want := []string{"_b2_0", "_b2_1", "_b2_2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stateIDs()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
