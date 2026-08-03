package export

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrmarble/termsvg/pkg/asciicast"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/progress"
	"github.com/mrmarble/termsvg/pkg/renderer"
	"github.com/mrmarble/termsvg/pkg/renderer/gif"
	"github.com/mrmarble/termsvg/pkg/renderer/svg"
	"github.com/mrmarble/termsvg/pkg/renderer/webm"
	"github.com/mrmarble/termsvg/pkg/theme"
	"github.com/tdewolff/minify/v2"
	msvg "github.com/tdewolff/minify/v2/svg"
)

type Cmd struct {
	File     string        `arg:"" type:"existingfile" help:"Asciicast file to export"`
	Output   string        `short:"o" type:"path" help:"Output file path (default: <input>.<format>)"`
	Format   string        `short:"f" default:"svg" enum:"svg,gif,webm" help:"Output format (svg, gif, webm)"`
	Minify   bool          `short:"m" help:"Minify output (SVG only)"`
	NoWindow bool          `short:"n" help:"Don't render terminal window chrome"`
	NoCursor bool          `short:"C" help:"Don't render cursor"`
	Speed    float64       `short:"s" default:"1.0" help:"Playback speed multiplier"`
	MaxIdle  time.Duration `short:"i" default:"0" help:"Cap idle time between frames (0 = unlimited)"`
	Cols     int           `short:"c" default:"0" help:"Override columns (0 = use original)"`
	Rows     int           `short:"r" default:"0" help:"Override rows (0 = use original)"`
	Debug    bool          `short:"d" help:"Enable debug logging"`
	Theme    string        `short:"t" help:"Theme name (built-in) or path to theme JSON file"`
}

type nbspWriter struct {
	w       io.Writer
	pending bool
}

func newNBSPWriter(w io.Writer) *nbspWriter { return &nbspWriter{w: w} }

func (w *nbspWriter) Write(p []byte) (int, error) {
	accepted := 0
	for _, b := range p {
		if w.pending {
			out := byte(0xc2)
			if b == 0xa0 {
				out = ' '
			}
			written, err := w.writeByte(out)
			if written {
				w.pending = false
				if b == 0xa0 {
					accepted++
				}
			}
			if err != nil {
				return accepted, err
			}
			if b == 0xa0 {
				continue
			}
		}
		if b == 0xc2 {
			w.pending = true
			accepted++
			continue
		}
		written, err := w.writeByte(b)
		if written {
			accepted++
		}
		if err != nil {
			return accepted, err
		}
	}
	return accepted, nil
}

func (w *nbspWriter) writeByte(b byte) (bool, error) {
	n, err := w.w.Write([]byte{b})
	if err == nil && n != 1 {
		err = io.ErrShortWrite
	}
	return n == 1, err
}

func (w *nbspWriter) Close() error {
	if !w.pending {
		return nil
	}
	written, err := w.writeByte(0xc2)
	if written {
		w.pending = false
	}
	return err
}

func writeOutput(
	ctx context.Context,
	rdr renderer.Renderer,
	rec *ir.Recording,
	dst io.Writer,
	minifySVG bool,
) error {
	if !minifySVG {
		return rdr.Render(ctx, rec, dst)
	}

	buf := bufio.NewWriter(dst)
	spaces := newNBSPWriter(buf)
	m := minify.New()
	m.AddFunc("image/svg+xml", msvg.Minify)
	minified := m.Writer("image/svg+xml", spaces)

	renderErr := rdr.Render(ctx, rec, minified)
	return errors.Join(renderErr, minified.Close(), spaces.Close(), buf.Flush())
}

//nolint:funlen,gocognit // sequential export steps are clearer in one function
func (cmd *Cmd) Run() error {
	format := strings.ToLower(cmd.Format)

	output := cmd.Output
	if output == "" {
		output = cmd.File + "." + format
	}

	// Load cast file
	f, err := os.Open(filepath.Clean(cmd.File))
	if err != nil {
		return err
	}
	defer f.Close()

	cast, err := asciicast.Parse(f)
	if err != nil {
		return err
	}

	// Override dimensions if specified
	if cmd.Cols > 0 {
		cast.Header.Width = cmd.Cols
	}
	if cmd.Rows > 0 {
		cast.Header.Height = cmd.Rows
	}

	// Determine theme to use
	selectedTheme := theme.Default()
	themeSource := "default"

	if cmd.Theme != "" {
		// CLI flag takes priority
		t, err := theme.Load(cmd.Theme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load theme %q: %v\n", cmd.Theme, err)
			fmt.Fprintf(os.Stderr, "Falling back to default theme\n")
		} else {
			selectedTheme = t
			themeSource = "CLI flag"
		}
	} else if cast.Header.Theme.Fg != "" {
		// Use theme from asciicast header
		t, err := theme.FromAsciinema("asciicast", cast.Header.Theme.Fg,
			cast.Header.Theme.Bg, cast.Header.Theme.Palette)
		if err != nil {
			if cmd.Debug {
				log.Printf("[Export] Invalid theme in asciicast header: %v", err)
			}
		} else {
			selectedTheme = t
			themeSource = "asciicast header"
		}
	}

	if cmd.Debug {
		log.Printf("[Export] Using theme from %s: %s", themeSource, selectedTheme.Name)
	}

	// Create progress reporter
	reporter, progressCh := progress.New()
	reporter.Start()
	exported := false
	defer func() {
		close(progressCh)
		reporter.Wait()
		if exported {
			fmt.Printf("\nExported: %s\n", output)
		}
	}()

	// Process through IR
	procConfig := ir.DefaultProcessorConfig()
	procConfig.Speed = cmd.Speed
	procConfig.IdleTimeLimit = cmd.MaxIdle
	procConfig.Theme = selectedTheme
	procConfig.ProgressCh = progressCh

	proc := ir.NewProcessor(procConfig)
	rec, err := proc.Process(cast)
	if err != nil {
		return err
	}

	// Create renderer based on format
	renderConfig := renderer.DefaultConfig()
	renderConfig.ShowWindow = !cmd.NoWindow
	renderConfig.ShowCursor = !cmd.NoCursor
	renderConfig.Minify = cmd.Minify
	renderConfig.Debug = cmd.Debug
	renderConfig.Theme = selectedTheme
	renderConfig.ProgressCh = progressCh

	var rdr renderer.Renderer
	switch format {
	case "gif":
		gifRenderer, err := gif.New(renderConfig)
		if err != nil {
			return fmt.Errorf("failed to create GIF renderer: %w", err)
		}
		rdr = gifRenderer
	case "svg":
		rdr = svg.New(renderConfig)
	case "webm":
		webmRenderer, err := webm.New(renderConfig)
		if err != nil {
			return fmt.Errorf("failed to create WebM renderer: %w", err)
		}
		rdr = webmRenderer
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	// Create output file
	outFile, err := os.Create(output) //nolint:gosec // output path is from user CLI input
	if err != nil {
		return err
	}
	defer outFile.Close()

	if err := writeOutput(context.Background(), rdr, rec, outFile, cmd.Minify && format == "svg"); err != nil {
		return err
	}

	exported = true
	return nil
}
