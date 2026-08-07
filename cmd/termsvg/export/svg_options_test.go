//nolint:lll // Compact option table rows are easier to compare.
package export

import (
	"testing"

	"github.com/mrmarble/termsvg/pkg/renderer/svg"
)

func TestNormalizedSVGOptions(t *testing.T) {
	tests := []struct {
		name    string
		cmd     Cmd
		format  string
		want    svg.Options
		wantErr bool
	}{
		{name: "zero value defaults", cmd: Cmd{}, format: "svg", want: svg.DefaultOptions()},
		{
			name: "regions layout", cmd: Cmd{SVGLayout: "REGIONS"}, format: "svg",
			want: svg.Options{Layout: svg.LayoutRegions, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveSize},
		},
		{
			name:   "experimental options",
			cmd:    Cmd{SVGLayout: "BANDS", SVGAnimation: "SMIL", SVGMaxFPS: 30},
			format: "svg",
			want: svg.Options{
				Layout:        svg.LayoutBands,
				Animation:     svg.AnimationSMIL,
				FrameSwitch:   svg.FrameSwitchTranslate,
				MaxFPS:        30,
				AutoObjective: svg.AutoObjectiveSize,
			},
		},
		{name: "runtime objective", cmd: Cmd{SVGLayout: "AUTO", SVGAutoObjective: "RUNTIME"}, format: "svg", want: svg.Options{Layout: svg.LayoutAuto, Animation: svg.AnimationCSS, FrameSwitch: svg.FrameSwitchTranslate, AutoObjective: svg.AutoObjectiveRuntime}},
		{name: "runtime requires auto", cmd: Cmd{SVGAutoObjective: "runtime"}, format: "svg", wantErr: true},
		{name: "invalid objective", cmd: Cmd{SVGLayout: "auto", SVGAutoObjective: "balanced"}, format: "svg", wantErr: true},
		{name: "non SVG rejects runtime", cmd: Cmd{SVGLayout: "auto", SVGAutoObjective: "runtime"}, format: "gif", wantErr: true},
		{name: "negative FPS", cmd: Cmd{SVGMaxFPS: -1}, format: "svg", wantErr: true},
		{name: "non SVG rejects non-default", cmd: Cmd{SVGLayout: "bands"}, format: "gif", wantErr: true},
		{
			name:   "non SVG accepts defaults",
			cmd:    Cmd{SVGLayout: "frames", SVGAnimation: "css"},
			format: "webm",
			want:   svg.DefaultOptions(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.cmd.normalizedSVGOptions(test.format)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizedSVGOptions() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("normalizedSVGOptions() = %#v, want %#v", got, test.want)
			}
		})
	}
}
