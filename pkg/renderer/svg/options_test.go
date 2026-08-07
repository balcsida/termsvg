//nolint:lll // Compact option literals make defaults easy to compare.
package svg

import (
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestDefaultOptionsPreserveCompatibilityPath(t *testing.T) {
	got := DefaultOptions()
	want := Options{Layout: LayoutFrames, Animation: AnimationCSS, FrameSwitch: FrameSwitchTranslate, AutoObjective: AutoObjectiveSize}
	if got != want {
		t.Fatalf("DefaultOptions() = %#v, want %#v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("default options are invalid: %v", err)
	}
}

func TestRegionsLayoutValidates(t *testing.T) {
	options := DefaultOptions()
	options.Layout = LayoutRegions
	if err := options.Validate(); err != nil {
		t.Fatalf("regions layout failed validation: %v", err)
	}
}

func TestRendererOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{name: "layout", options: Options{Layout: "columns", Animation: AnimationCSS}},
		{name: "animation", options: Options{Layout: LayoutFrames, Animation: "script"}},
		{name: "objective", options: Options{Layout: LayoutAuto, Animation: AnimationCSS, AutoObjective: "balanced"}},
		{name: "runtime requires auto", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, AutoObjective: AutoObjectiveRuntime}},
		{name: "negative FPS", options: Options{Layout: LayoutFrames, Animation: AnimationCSS, MaxFPS: -1}},
		{
			name: "zero duration bucket",
			options: Options{
				Layout: LayoutFrames, Animation: AnimationCSS, MaxFPS: int(time.Second) + 1,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.options.Validate(); err == nil {
				t.Fatalf("Validate(%#v) succeeded", test.options)
			}
		})
	}
}

func TestNewAcceptsFunctionalOptionsWithoutChangingExistingCallers(t *testing.T) {
	config := renderer.DefaultConfig()
	if got := New(config); got.options != DefaultOptions() {
		t.Fatalf("New(config) options = %#v", got.options)
	}
	got := New(config, WithLayout(LayoutAuto), WithAnimation(AnimationSMIL), WithMaxFPS(30), WithAutoObjective(AutoObjectiveRuntime))
	want := Options{
		Layout: LayoutAuto, Animation: AnimationSMIL, FrameSwitch: FrameSwitchTranslate, MaxFPS: 30,
		AutoObjective: AutoObjectiveRuntime,
	}
	if got.options != want {
		t.Fatalf("functional options = %#v, want %#v", got.options, want)
	}
}
