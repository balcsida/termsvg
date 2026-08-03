// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes to translate between frames.
package svg

import (
	"bufio"
	"context"
	"fmt"
	"html"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config renderer.Config
}

// canvas holds rendering state
type canvas struct {
	w          io.Writer
	rec        *ir.Recording
	config     renderer.Config
	classNames map[color.ID]string
}

type renderedRow struct {
	svg   string
	count int
	id    string
}

type backgroundSpan struct {
	startCol int
	endCol   int
	colorID  color.ID
}

// Layout constants for SVG rendering
const (
	RowHeight  = 25 // pixels per row
	ColWidth   = 12 // pixels per column
	Padding    = 20 // padding around content
	HeaderSize = 2  // multiplier for header area (window buttons)

	// windowCornerRadius is the radius for rounded window corners.
	windowCornerRadius = 5

	// windowButtonSpacing is the horizontal spacing between window buttons.
	windowButtonSpacing = 20

	// windowButtonRadius is the radius of window control buttons.
	windowButtonRadius = 6
)

// New creates a new SVG renderer with the given configuration.
func New(config *renderer.Config) *Renderer {
	return &Renderer{config: *config}
}

// Format returns the output format name.
func (r *Renderer) Format() string {
	return "svg"
}

// FileExtension returns the file extension for SVG files.
func (r *Renderer) FileExtension() string {
	return ".svg"
}

// Render generates an animated SVG from the recording.
func (r *Renderer) Render(ctx context.Context, rec *ir.Recording, w io.Writer) error {
	if len(rec.Frames) == 0 {
		return fmt.Errorf("recording has no frames")
	}

	buf := bufio.NewWriterSize(w, 64*1024)
	c := &canvas{
		w:          buf,
		rec:        rec,
		config:     r.config,
		classNames: rec.Colors.GenerateClassNames(),
	}

	if err := c.render(ctx); err != nil {
		return err
	}
	return buf.Flush()
}

func (c *canvas) contentWidth() int {
	return c.rec.Width * ColWidth
}

func (c *canvas) contentHeight() int {
	return c.rec.Height * RowHeight
}

func (c *canvas) paddedWidth() int {
	return c.contentWidth() + 2*Padding
}

func (c *canvas) paddedHeight() int {
	if c.config.ShowWindow {
		return c.contentHeight() + Padding*HeaderSize + Padding
	}
	return c.contentHeight() + 2*Padding
}

func (c *canvas) render(ctx context.Context) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	frameRows, rowDefs := c.collectRows()

	// SVG header
	width := c.paddedWidth()
	height := c.paddedHeight()
	fmt.Fprintf(c.w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`, width, height)

	if c.config.ShowWindow {
		c.writeWindow()
	} else {
		c.writeBackground()
	}

	// Content group with clipping
	contentY := Padding
	if c.config.ShowWindow {
		contentY = Padding * HeaderSize
	}

	fmt.Fprintf(c.w, `<defs><clipPath id="clip"><rect width="%d" height="%d"/></clipPath>`,
		c.contentWidth(), c.contentHeight())
	c.writeRowDefs(rowDefs)
	fmt.Fprint(c.w, `</defs>`)

	fmt.Fprintf(c.w, `<g transform="translate(%d,%d)" clip-path="url(#clip)">`, Padding, contentY)

	c.writeStyles()

	// Animation group
	duration := c.rec.Duration.Seconds()
	loopAttr := "infinite"
	if c.config.LoopCount == -1 {
		loopAttr = "1"
	} else if c.config.LoopCount > 0 {
		loopAttr = fmt.Sprintf("%d", c.config.LoopCount)
	}

	fmt.Fprintf(c.w, `<g style="animation:k %.3fs %s steps(1,end)">`, duration, loopAttr)

	c.writeFrames(frameRows)

	fmt.Fprint(c.w, `</g></g></svg>`)

	return nil
}

func (c *canvas) writeBackground() {
	bgHex := color.RGBAtoHex(c.config.Theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect width="100%%" height="100%%" fill="%s"/>`, bgHex)
}

func (c *canvas) writeWindow() {
	theme := c.config.Theme

	// Window background with rounded corners
	bgHex := color.RGBAtoHex(theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect rx="%d" width="100%%" height="100%%" fill="%s"/>`, windowCornerRadius, bgHex)

	// Window buttons (close, minimize, maximize)
	buttonY := Padding
	for i, btnColor := range theme.WindowButtons {
		btnHex := color.RGBAtoHex(btnColor)
		x := Padding + i*windowButtonSpacing
		fmt.Fprintf(c.w, `<circle cx="%d" cy="%d" r="%d" fill="%s"/>`, x, buttonY, windowButtonRadius, btnHex)
	}
}

func (c *canvas) writeStyles() {
	var sb strings.Builder
	sb.WriteString("<style>")

	// Keyframes animation
	sb.WriteString(c.generateKeyframes())

	// Cursor blink animation
	sb.WriteString("@keyframes blink{0%,50%{opacity:1}50.01%,100%{opacity:0}}")

	// Default text style (white-space:pre preserves spaces, survives minification)
	fgHex := color.RGBAtoHex(c.rec.Colors.DefaultForeground())
	fmt.Fprintf(&sb, "text{font-family:%s;font-size:%dpx;fill:%s;white-space:pre}",
		c.config.FontFamily, c.config.FontSize, fgHex)

	// Cursor style
	fmt.Fprintf(&sb, ".cursor{fill:%s;animation:blink 1s step-end infinite}", fgHex)

	// Color classes
	for _, id := range c.visibleColorIDs() {
		rgba := c.rec.Colors.Resolved(id)
		className := c.classNames[id]
		hex := color.RGBAtoHex(rgba)
		fmt.Fprintf(&sb, ".%s{fill:%s}", className, hex)
	}

	// Attribute classes (only if used)
	if c.rec.Stats.HasBold {
		sb.WriteString(".bold{font-weight:bold}")
	}
	if c.rec.Stats.HasItalic {
		sb.WriteString(".italic{font-style:italic}")
	}
	if c.rec.Stats.HasUnderline {
		sb.WriteString(".underline{text-decoration:underline}")
	}
	if c.rec.Stats.HasDim {
		sb.WriteString(".dim{opacity:0.5}")
	}

	sb.WriteString("</style>")
	fmt.Fprint(c.w, sb.String())
}

func (c *canvas) visibleColorIDs() []color.ID {
	used := make(map[color.ID]bool)
	for _, frame := range c.rec.Frames {
		for _, row := range frame.Rows {
			for _, run := range row.Runs {
				if !c.rec.Colors.IsDefault(run.Attrs.BG) && runEndCol(run) > run.StartCol {
					used[run.Attrs.BG] = true
				}
				if shouldRenderText(run) && !c.rec.Colors.IsDefault(run.Attrs.FG) {
					used[run.Attrs.FG] = true
				}
			}
		}
	}
	ids := make([]color.ID, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (c *canvas) generateKeyframes() string {
	if len(c.rec.Frames) <= 1 {
		return "@keyframes k{0%{transform:translateX(0)}}"
	}

	var sb strings.Builder
	sb.WriteString("@keyframes k{")

	duration := c.rec.Duration.Seconds()
	pw := c.paddedWidth()

	for i, frame := range c.rec.Frames {
		pct := frame.Time.Seconds() / duration * 100
		offset := -pw * i
		fmt.Fprintf(&sb, "%.3f%%{transform:translateX(%dpx)}", pct, offset)
	}

	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) collectRows() ([][]*renderedRow, []*renderedRow) {
	seen := make(map[string]*renderedRow)
	frames := make([][]*renderedRow, len(c.rec.Frames))
	ordered := make([]*renderedRow, 0)

	for i, frame := range c.rec.Frames {
		for _, row := range frame.Rows {
			var sb strings.Builder
			c.writeRow(&sb, row)
			markup := sb.String()
			entry := seen[markup]
			if entry == nil {
				entry = &renderedRow{svg: markup}
				seen[markup] = entry
				ordered = append(ordered, entry)
			}
			entry.count++
			frames[i] = append(frames[i], entry)
		}
	}

	defs := make([]*renderedRow, 0)
	for _, entry := range ordered {
		id := fmt.Sprintf("r%d", len(defs))
		use := fmt.Sprintf(`<use href="#%s"/>`, id)
		inlineBytes := entry.count * len(entry.svg)
		definitionBytes := len(`<g id="">`) + len(id) + len(entry.svg) + len(`</g>`) + entry.count*len(use)
		if definitionBytes < inlineBytes {
			entry.id = id
			defs = append(defs, entry)
		}
	}

	return frames, defs
}

func (c *canvas) writeRowDefs(defs []*renderedRow) {
	for _, row := range defs {
		fmt.Fprintf(c.w, `<g id="%s">%s</g>`, row.id, row.svg)
	}
}

func (c *canvas) writeFrames(frameRows [][]*renderedRow) {
	pw := c.paddedWidth()
	for i, frame := range c.rec.Frames {
		offset := pw * i
		fmt.Fprintf(c.w, `<g transform="translate(%d,0)">`, offset)
		for _, row := range frameRows[i] {
			if row.id != "" {
				fmt.Fprintf(c.w, `<use href="#%s"/>`, row.id)
			} else {
				fmt.Fprint(c.w, row.svg)
			}
		}

		// Render cursor if visible and cursor rendering is enabled
		if c.config.ShowCursor && frame.Cursor.Visible {
			c.writeCursor(frame.Cursor)
		}
		fmt.Fprint(c.w, "</g>")
	}
}

func (c *canvas) writeRow(w io.Writer, row ir.Row) {
	for _, span := range c.backgroundSpans(row) {
		fmt.Fprintf(w, `<rect class="%s" x="%d" y="%d" width="%d" height="%d"/>`,
			c.classNames[span.colorID], span.startCol*ColWidth, row.Y*RowHeight,
			(span.endCol-span.startCol)*ColWidth, RowHeight)
	}
	for _, run := range row.Runs {
		c.writeTextRun(w, run, row.Y)
	}
}

func (c *canvas) backgroundSpans(row ir.Row) []backgroundSpan {
	spans := make([]backgroundSpan, 0, len(row.Runs))
	for _, run := range row.Runs {
		endCol := runEndCol(run)
		if c.rec.Colors.IsDefault(run.Attrs.BG) || endCol <= run.StartCol {
			continue
		}
		if len(spans) > 0 && spans[len(spans)-1].colorID == run.Attrs.BG && spans[len(spans)-1].endCol == run.StartCol {
			spans[len(spans)-1].endCol = endCol
			continue
		}
		spans = append(spans, backgroundSpan{startCol: run.StartCol, endCol: endCol, colorID: run.Attrs.BG})
	}
	return spans
}

func runEndCol(run ir.TextRun) int {
	if run.EndCol > run.StartCol {
		return run.EndCol
	}
	return run.StartCol + utf8.RuneCountInString(run.Text)
}

func shouldRenderText(run ir.TextRun) bool {
	return run.Text != "" && (strings.TrimSpace(run.Text) != "" || run.Attrs.Underline)
}

func (c *canvas) writeCursor(cursor ir.Cursor) {
	x := cursor.Col * ColWidth
	y := cursor.Row * RowHeight

	// Render cursor as a rectangle (block cursor)
	fmt.Fprintf(c.w, `<rect class="cursor" x="%d" y="%d" width="%d" height="%d"/>`,
		x, y, ColWidth, RowHeight)
}

func (c *canvas) writeTextRun(w io.Writer, run ir.TextRun, rowY int) {
	if !shouldRenderText(run) {
		return
	}

	// Replace spaces with non-breaking spaces to survive minification
	// Only needed when minifying, as the minifier strips regular spaces
	text := run.Text
	if c.config.Minify {
		text = strings.ReplaceAll(text, " ", "\u00A0")
	}

	x := run.StartCol * ColWidth
	y := (rowY*RowHeight + RowHeight) - 5 // baseline offset

	// Build class list
	var classes []string
	if !c.rec.Colors.IsDefault(run.Attrs.FG) {
		classes = append(classes, c.classNames[run.Attrs.FG])
	}
	if run.Attrs.Bold {
		classes = append(classes, "bold")
	}
	if run.Attrs.Italic {
		classes = append(classes, "italic")
	}
	if run.Attrs.Underline {
		classes = append(classes, "underline")
	}
	if run.Attrs.Dim {
		classes = append(classes, "dim")
	}

	// Build attributes
	classAttr := ""
	if len(classes) > 0 {
		classAttr = fmt.Sprintf(" class=%q", strings.Join(classes, " "))
	}

	fmt.Fprintf(w, `<text x="%d" y="%d" xml:space="preserve"%s>%s</text>`,
		x, y, classAttr, html.EscapeString(text))
}
