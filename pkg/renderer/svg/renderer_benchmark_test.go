package svg

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	termcolor "github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

func BenchmarkRender(b *testing.B) {
	b.Run("small_two_frame", func(b *testing.B) { benchmarkRecording(b, createTestRecording()) })
	b.Run("many_frames_same_static_rows", func(b *testing.B) { benchmarkRecording(b, staticFrames(200)) })
	b.Run("cursor_only_changes", func(b *testing.B) { benchmarkRecording(b, cursorFrames(200)) })
	b.Run("color_background_heavy", func(b *testing.B) { benchmarkRecording(b, colorFrames()) })
	b.Run("repeated_dynamic_rows", func(b *testing.B) { benchmarkRecording(b, repeatedRows(200)) })
	benchmarkCast(b, "256colors.cast")
	benchmarkCast(b, "444816.cast")
}

func benchmarkRecording(b *testing.B, rec *ir.Recording) {
	b.Helper()
	r := New(renderer.DefaultConfig())
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := r.Render(ctx, rec, io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCast(b *testing.B, name string) {
	b.Helper()
	b.Run(name, func(b *testing.B) {
		path := filepath.Join("..", "..", "..", "examples", name)
		f, err := os.Open(path) //nolint:gosec // repository benchmark fixture
		if os.IsNotExist(err) {
			b.Skipf("optional fixture not found: %s", path)
		}
		if err != nil {
			b.Fatal(err)
		}
		cast, err := asciicast.Parse(f)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			b.Fatal(err)
		}
		rec, err := ir.NewProcessor(ir.DefaultProcessorConfig()).Process(cast)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkRecording(b, rec)
	})
}

func staticFrames(count int) *ir.Recording {
	rec := createTestRecording()
	rows := []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: "unchanged output"}}}}
	rec.Frames = make([]ir.Frame, count)
	for i := range rec.Frames {
		rec.Frames[i] = ir.Frame{Time: time.Duration(i) * time.Millisecond, Index: i, Rows: rows}
	}
	rec.Duration = time.Duration(count) * time.Millisecond
	return rec
}

func cursorFrames(count int) *ir.Recording {
	rec := staticFrames(count)
	for i := range rec.Frames {
		rec.Frames[i].Cursor = ir.Cursor{Col: i % rec.Width, Row: i % rec.Height, Visible: true}
	}
	return rec
}

func colorFrames() *ir.Recording {
	rec := createTestRecording()
	palette := termcolor.Standard()
	rows := make([]ir.Row, 16)
	for i := range rows {
		fg := rec.Colors.Register(termcolor.FromRGB(uint8(i*15), uint8(255-i*12), uint8(i*7)), &palette)
		bg := rec.Colors.Register(termcolor.FromRGB(uint8(255-i*9), uint8(i*13), uint8(128+i)), &palette)
		rows[i] = ir.Row{Y: i, Runs: []ir.TextRun{{Text: fmt.Sprintf("color %02d", i), EndCol: 8, Attrs: ir.CellAttrs{FG: fg, BG: bg}}}}
	}
	rec.Frames = []ir.Frame{{Rows: rows}}
	rec.Duration = time.Second
	rec.Stats.HasTrueColor = true
	return rec
}

func repeatedRows(count int) *ir.Recording {
	rec := createTestRecording()
	variants := [4][]ir.Row{}
	for i := range variants {
		variants[i] = []ir.Row{{Y: 0, Runs: []ir.TextRun{{Text: fmt.Sprintf("dynamic row %d", i)}}}}
	}
	rec.Frames = make([]ir.Frame, count)
	for i := range rec.Frames {
		rec.Frames[i] = ir.Frame{Time: time.Duration(i) * time.Millisecond, Index: i, Rows: variants[i%len(variants)]}
	}
	rec.Duration = time.Duration(count) * time.Millisecond
	return rec
}
