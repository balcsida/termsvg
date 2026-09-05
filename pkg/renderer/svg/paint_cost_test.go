package svg

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func TestNamedPaintCostsMatchFinalMinifiedExamples(t *testing.T) {
	for _, test := range []struct {
		name   string
		layout LayoutMode
		fps    int
	}{
		{"256colors.cast", LayoutBands, 0},
		{"htop.cast", LayoutFrames, 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := loadRegionCast(t, test.name)
			config := renderer.DefaultConfig()
			config.Minify = true
			r := New(config, WithLayout(test.layout), WithAnimation(AnimationSMIL),
				WithFrameSwitch(FrameSwitchHref), WithMaxFPS(test.fps), WithStyleMode(StyleAuto))
			metrics, err := r.MeasureCandidate(context.Background(), rec)
			if err != nil {
				t.Fatal(err)
			}
			var final bytes.Buffer
			if err := svgoutput.Write(&final, func(w io.Writer) error {
				return r.Render(context.Background(), rec, w)
			}); err != nil {
				t.Fatal(err)
			}
			if metrics.FinalBytes != int64(final.Len()) {
				t.Fatalf("measured %d bytes; final minified output %d", metrics.FinalBytes, final.Len())
			}
		})
	}
}
