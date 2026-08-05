// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config  renderer.Config
	options Options
}

// canvas holds rendering state
type canvas struct {
	w          io.Writer
	rec        *ir.Recording
	plan       renderPlan
	config     renderer.Config
	options    Options
	classNames map[color.ID]string
	metrics    *CandidateMetrics
}

type renderedRow struct {
	row        ir.Row
	svg        string
	definition string
	count      int
	order      int
	id         string
}

type backgroundSpan struct {
	startCol int
	endCol   int
	colorID  color.ID
}

type countingWriter struct {
	bytes        int64
	collapseNBSP bool
	pendingC2    bool
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

var svgTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// New creates a new SVG renderer with the given configuration.
func New(config *renderer.Config, opts ...Option) *Renderer {
	options := DefaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &Renderer{config: *config, options: options.normalized()}
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
	if err := r.options.Validate(); err != nil {
		return err
	}
	if r.options.Layout == LayoutAuto {
		layout, err := r.selectAutoLayout(ctx, rec)
		if err != nil {
			return err
		}
		options := r.options
		options.Layout = layout
		return r.renderWithOptions(ctx, rec, w, options)
	}
	return r.renderWithOptions(ctx, rec, w, r.options)
}

func (r *Renderer) selectAutoLayout(ctx context.Context, rec *ir.Recording) (LayoutMode, error) {
	best := LayoutFrames
	bestBytes := int64(-1)
	for _, layout := range []LayoutMode{LayoutFrames, LayoutBands} {
		options := r.options
		options.Layout = layout
		metrics, err := r.renderCandidate(ctx, rec, io.Discard, options)
		if err != nil {
			return "", err
		}
		// Frames win deterministic ties because they are the compatibility
		// layout and are measured first.
		if bestBytes < 0 || metrics.FinalBytes < bestBytes {
			best = layout
			bestBytes = metrics.FinalBytes
		}
	}
	return best, nil
}

// MeasureCandidate renders the configured candidate to a sink and reports its
// exact serialized size and structural cost.
func (r *Renderer) MeasureCandidate(ctx context.Context, rec *ir.Recording) (CandidateMetrics, error) {
	if len(rec.Frames) == 0 {
		return CandidateMetrics{}, fmt.Errorf("recording has no frames")
	}
	if err := r.options.Validate(); err != nil {
		return CandidateMetrics{}, err
	}
	options := r.options
	if options.Layout == LayoutAuto {
		layout, err := r.selectAutoLayout(ctx, rec)
		if err != nil {
			return CandidateMetrics{}, err
		}
		options.Layout = layout
	}
	return r.renderCandidate(ctx, rec, io.Discard, options)
}

func (r *Renderer) renderWithOptions(
	ctx context.Context,
	rec *ir.Recording,
	w io.Writer,
	options Options,
) error {
	_, err := r.renderCandidate(ctx, rec, w, options)
	return err
}

func (r *Renderer) renderCandidate(
	ctx context.Context,
	rec *ir.Recording,
	w io.Writer,
	options Options,
) (CandidateMetrics, error) {
	metrics := CandidateMetrics{}
	candidate := &candidateWriter{w: w, metrics: &metrics, collapseNBSP: r.config.Minify}
	buf := bufio.NewWriterSize(candidate, 64*1024)
	plan := buildRenderPlanWithOptions(rec, r.config.ShowCursor, options)
	plan.pruneZeroDwellCursorEndpoint(r.config.LoopCount)
	c := &canvas{
		w:          buf,
		rec:        rec,
		plan:       plan,
		config:     r.config,
		options:    options,
		classNames: rec.Colors.GenerateClassNames(),
		metrics:    &metrics,
	}

	if err := c.render(ctx); err != nil {
		return metrics, err
	}
	if err := buf.Flush(); err != nil {
		return metrics, err
	}
	candidate.finish()
	return metrics, nil
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if !w.collapseNBSP {
		w.bytes += int64(len(p))
		return len(p), nil
	}
	for _, value := range p {
		if w.pendingC2 {
			if value == 0xa0 {
				w.bytes++
				w.pendingC2 = false
				continue
			}
			w.bytes++
			w.pendingC2 = false
		}
		if value == 0xc2 {
			w.pendingC2 = true
		} else {
			w.bytes++
		}
	}
	return len(p), nil
}

func (w *countingWriter) size() int64 {
	if w.pendingC2 {
		return w.bytes + 1
	}
	return w.bytes
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

	content := c.prepareContent()
	addPreparedMetrics(c.metrics, &content, c.options, c.contentWidth(), c.contentHeight())

	// SVG header
	width := c.paddedWidth()
	height := c.paddedHeight()
	fmt.Fprintf(c.w, `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="%d" height="%d">`, width, height)

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
	c.writeRowDefs(content.rowDefs)
	c.writeStateDefs(&content)
	fmt.Fprint(c.w, `</defs>`)

	fmt.Fprintf(c.w, `<g transform="translate(%d,%d)" clip-path="url(#clip)">`, Padding, contentY)

	c.writeStyles(&content)
	for _, row := range c.plan.staticRows {
		c.writeRow(c.w, row)
	}
	c.writeContent(&content)
	c.writeCursor()

	fmt.Fprint(c.w, `</g></svg>`)

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

func (c *canvas) writeStyles(content *preparedContent) {
	var sb strings.Builder
	sb.WriteString("<style>")

	if c.options.Animation == AnimationCSS {
		if c.options.Layout == LayoutBands {
			written := make(map[string]bool)
			for _, band := range content.bands {
				if band.name == "" || written[band.name] {
					continue
				}
				sb.WriteString(c.generateBandKeyframes(band.name, band.keyframes, band.width*ColWidth))
				written[band.name] = true
			}
		} else if len(content.frameKeyframes) > 1 {
			sb.WriteString(c.generateKeyframes(content.frameKeyframes))
		}
		if len(c.cursorKeyframes()) > 1 {
			sb.WriteString(c.generateCursorKeyframes())
		}
	}
	if c.plan.cursorEverVisible {
		sb.WriteString("@keyframes blink{0%,50%{opacity:1}50.01%,100%{opacity:0}}")
	}

	// Default text style (white-space:pre preserves spaces, survives minification)
	fgHex := color.RGBAtoHex(c.rec.Colors.DefaultForeground())
	fmt.Fprintf(&sb, "text{font-family:%s;font-size:%dpx;fill:%s;white-space:pre}",
		c.config.FontFamily, c.config.FontSize, fgHex)

	if c.plan.cursorEverVisible {
		fmt.Fprintf(&sb, ".cursor{fill:%s;animation:blink 1s step-end infinite}", fgHex)
	}

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
	return c.plan.usedColors
}

func (c *canvas) generateKeyframes(frames []keyframePoint[int]) string {
	var sb strings.Builder
	sb.WriteString("@keyframes k{")
	width := c.contentWidth()
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{transform:translateX(%dpx)}", frame.selector, -width*frame.state)
	}

	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) contentKeyframes() ([]keyframePoint[int], [][]ir.Row) {
	return contentKeyframesFor(c.plan.content)
}

func (c *canvas) collectRows(contentStates [][]ir.Row) (frames [][]*renderedRow, defs []*renderedRow) {
	return c.collectRowsWithHash(contentStates, semanticRowHash)
}

func (c *canvas) collectRowsWithHash(
	contentStates [][]ir.Row,
	hash func(ir.Row) uint64,
) (frames [][]*renderedRow, defs []*renderedRow) {
	seen := make(map[uint64][]*renderedRow)
	frames = make([][]*renderedRow, len(contentStates))
	ordered := make([]*renderedRow, 0)

	for i, rows := range contentStates {
		for _, row := range rows {
			key := hash(row)
			var entry *renderedRow
			for _, candidate := range seen[key] {
				if rowEqual(candidate.row, row) {
					entry = candidate
					break
				}
			}
			if entry == nil {
				entry = &renderedRow{row: row, order: len(ordered)}
				seen[key] = append(seen[key], entry)
				ordered = append(ordered, entry)
			}
			entry.count++
			frames[i] = append(frames[i], entry)
		}
	}

	for _, entry := range ordered {
		var sb strings.Builder
		c.writeRow(&sb, entry.row)
		entry.svg = sb.String()
	}

	// Give the most frequently referenced rows the shortest identifiers. The
	// first-occurrence order is the deterministic tie-breaker.
	candidates := append([]*renderedRow(nil), ordered...)
	slices.SortStableFunc(candidates, func(a, b *renderedRow) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return a.order - b.order
	})

	defs = make([]*renderedRow, 0)
	for _, entry := range candidates {
		if entry.count < 2 || entry.svg == "" {
			continue
		}
		id := compactXMLID(len(defs))
		definition := c.rowDefinition(entry, id)
		use := `<use href="#` + id + `"/>`
		inlineBytes := entry.count * finalSVGBytes(entry.svg, c.config.Minify)
		definitionBytes := finalSVGBytes(definition, c.config.Minify) +
			entry.count*finalSVGBytes(use, c.config.Minify)
		if definitionBytes < inlineBytes {
			entry.id = id
			entry.definition = definition
			defs = append(defs, entry)
		}
	}

	return frames, defs
}

func compactXMLID(index int) string {
	for {
		value := compactXMLIDAt(index)
		if value != "clip" {
			return value
		}
		index++
	}
}

func compactXMLIDAt(index int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	index++
	var id [16]byte
	pos := len(id)
	for index > 0 {
		index--
		pos--
		id[pos] = alphabet[index%len(alphabet)]
		index /= len(alphabet)
	}
	return string(id[pos:])
}

func finalSVGBytes(value string, minified bool) int {
	bytes := len(value)
	if minified {
		// The export pipeline converts every UTF-8 NBSP (two bytes) back to a
		// one-byte ASCII space after minification. Profitability must therefore
		// use the bytes that will actually reach the output file.
		bytes -= strings.Count(value, "\u00a0")
	}
	return bytes
}

func (c *canvas) rowDefinition(row *renderedRow, id string) string {
	if c.rowElementCount(row.row) == 1 {
		return addElementID(row.svg, id)
	}
	return `<g id="` + id + `">` + row.svg + `</g>`
}

func addElementID(svg, id string) string {
	for _, prefix := range []string{"<rect", "<text"} {
		if strings.HasPrefix(svg, prefix) {
			return prefix + ` id="` + id + `"` + strings.TrimPrefix(svg, prefix)
		}
	}
	return `<g id="` + id + `">` + svg + `</g>`
}

func (c *canvas) rowElementCount(row ir.Row) int {
	count := len(c.backgroundSpans(row))
	for _, run := range row.Runs {
		if _, _, ok := compactTextRun(run); ok {
			count++
		}
	}
	return count
}

func semanticRowHash(row ir.Row) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	addInt := func(value int) {
		//nolint:gosec // convert signed values to stable two's-complement hash bytes.
		v := uint64(value)
		for range 8 {
			h = (h ^ uint64(byte(v))) * prime
			v >>= 8
		}
	}
	addInt(row.Y)
	addInt(len(row.Runs))
	for _, run := range row.Runs {
		addInt(len(run.Text))
		for i := range len(run.Text) {
			h = (h ^ uint64(run.Text[i])) * prime
		}
		addInt(run.StartCol)
		addInt(run.EndCol)
		addInt(int(run.Attrs.FG))
		addInt(int(run.Attrs.BG))
		flags := uint64(0)
		if run.Attrs.Bold {
			flags |= 1
		}
		if run.Attrs.Italic {
			flags |= 2
		}
		if run.Attrs.Underline {
			flags |= 4
		}
		if run.Attrs.Dim {
			flags |= 8
		}
		h = (h ^ flags) * prime
	}
	return h
}

func (c *canvas) writeRowDefs(defs []*renderedRow) {
	for _, row := range defs {
		fmt.Fprint(c.w, row.definition)
	}
}

func (c *canvas) writeStateDefs(content *preparedContent) {
	if c.options.FrameSwitch != FrameSwitchHref {
		return
	}
	for i, id := range content.frameStateIDs {
		fmt.Fprintf(c.w, `<g id="%s">`, id)
		c.writeFrameRows(content.frameRows[i])
		fmt.Fprint(c.w, `</g>`)
	}
	for _, band := range content.bands {
		for i, id := range band.stateIDs {
			fmt.Fprintf(c.w, `<g id="%s">`, id)
			c.writeFrameRows(band.rows[i])
			fmt.Fprint(c.w, `</g>`)
		}
	}
}

func (c *canvas) writeContent(content *preparedContent) {
	if c.options.Layout == LayoutBands {
		c.writeBands(content.bands)
		return
	}
	c.writeFrames(content.frameRows, content.frameKeyframes, content.frameStateIDs)
}

func (c *canvas) writeFrames(
	frameRows [][]*renderedRow,
	frames []keyframePoint[int],
	stateIDs []string,
) {
	if len(frames) <= 1 {
		if len(frameRows) > 0 {
			c.writeFrameRows(frameRows[len(frameRows)-1])
		}
		return
	}
	if c.options.FrameSwitch == FrameSwitchHref {
		c.writeHrefSequence(frames, stateIDs)
		return
	}
	if c.options.Animation == AnimationSMIL {
		fmt.Fprint(c.w, `<g>`)
		c.writeSMILTranslate(c.w, frames, c.contentWidth())
	} else {
		fmt.Fprintf(c.w, `<g style="animation:k %s %s step-end">`, animationDuration(c.plan.duration), c.loopCount())
	}
	c.writeStateStrip(frameRows, c.contentWidth())
	fmt.Fprint(c.w, "</g>")
}

func (c *canvas) writeBands(bands []preparedBand) {
	for _, band := range bands {
		width := band.width * ColWidth
		height := band.height * RowHeight
		fmt.Fprintf(c.w, `<svg x="%d" y="%d" width="%d" height="%d" overflow="hidden">`,
			band.x*ColWidth, band.y*RowHeight, width, height)
		if len(band.keyframes) <= 1 {
			if len(band.rows) > 0 {
				c.writeFrameRows(band.rows[len(band.rows)-1])
			}
			fmt.Fprint(c.w, `</svg>`)
			continue
		}
		if c.options.FrameSwitch == FrameSwitchHref {
			c.writeHrefSequence(band.keyframes, band.stateIDs)
			fmt.Fprint(c.w, `</svg>`)
			continue
		}
		if c.options.Animation == AnimationSMIL {
			fmt.Fprint(c.w, `<g>`)
			c.writeSMILTranslate(c.w, band.keyframes, width)
		} else {
			fmt.Fprintf(c.w, `<g style="animation:%s %s %s step-end">`,
				band.name, animationDuration(c.plan.duration), c.loopCount())
		}
		c.writeStateStrip(band.rows, width)
		fmt.Fprint(c.w, `</g></svg>`)
	}
}

func (c *canvas) writeHrefSequence(frames []keyframePoint[int], ids []string) {
	if len(frames) == 0 || len(ids) == 0 {
		return
	}
	initial := frames[0].state
	if initial < 0 || initial >= len(ids) {
		return
	}
	fmt.Fprintf(c.w, `<use href="#%s">`, ids[initial])
	c.writeSMILHref(c.w, frames, ids)
	fmt.Fprint(c.w, `</use>`)
}

func (c *canvas) writeStateStrip(states [][]*renderedRow, width int) {
	for i, rows := range states {
		fmt.Fprintf(c.w, `<g transform="translate(%d)">`, width*i)
		c.writeFrameRows(rows)
		fmt.Fprint(c.w, `</g>`)
	}
}

func (c *canvas) generateBandKeyframes(name string, frames []keyframePoint[int], width int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "@keyframes %s{", name)
	for _, frame := range frames {
		fmt.Fprintf(&sb, "%s{transform:translateX(%dpx)}", frame.selector, -width*frame.state)
	}
	sb.WriteString("}")
	return sb.String()
}

func (c *canvas) writeFrameRows(rows []*renderedRow) {
	for _, row := range rows {
		if row.id != "" {
			fmt.Fprintf(c.w, `<use href="#%s"/>`, row.id)
		} else {
			fmt.Fprint(c.w, row.svg)
		}
	}
}

func (c *canvas) writeRow(w io.Writer, row ir.Row) {
	for _, span := range c.backgroundSpans(row) {
		x := span.startCol * ColWidth
		xAttr := ""
		if x != 0 {
			xAttr = fmt.Sprintf(` x="%d"`, x)
		}
		fmt.Fprintf(w, `<rect class="%s"%s y="%d" width="%d" height="%d"/>`,
			c.classNames[span.colorID], xAttr, row.Y*RowHeight,
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
	_, _, ok := compactTextRun(run)
	return ok
}

// compactTextRun removes only visually inert ASCII spaces. Backgrounds are
// emitted independently, and underlined whitespace remains visible. Other
// Unicode whitespace is intentionally preserved.
func compactTextRun(run ir.TextRun) (text string, startCol int, ok bool) {
	text = run.Text
	startCol = run.StartCol
	if !run.Attrs.Underline {
		trimmedLeft := strings.TrimLeft(text, " ")
		startCol += len(text) - len(trimmedLeft)
		text = strings.TrimRight(trimmedLeft, " ")
	}
	return text, startCol, text != ""
}

func (c *canvas) writeCursor() {
	if !c.plan.cursorEverVisible {
		return
	}
	point := c.plan.cursor.points[len(c.plan.cursor.points)-1]
	style := ""
	frames := c.cursorKeyframes()
	if len(frames) > 0 {
		point.state = frames[0].state
	}
	if len(frames) > 1 && c.options.Animation == AnimationCSS {
		style = fmt.Sprintf(` style="animation:cursor %s %s step-end"`, animationDuration(c.plan.duration), c.loopCount())
	}
	fmt.Fprintf(c.w, `<g transform="translate(%d,%d)" visibility="%s"%s>`,
		point.state.Col*ColWidth, point.state.Row*RowHeight, cursorVisibility(point.state), style)
	if len(frames) > 1 && c.options.Animation == AnimationSMIL {
		c.writeSMILCursor(c.w, frames)
	}
	fmt.Fprintf(c.w, `<rect class="cursor" width="%d" height="%d"/></g>`, ColWidth, RowHeight)
}

func (c *canvas) writeTextRun(w io.Writer, run ir.TextRun, rowY int) {
	text, startCol, ok := compactTextRun(run)
	if !ok {
		return
	}

	// Replace spaces with non-breaking spaces to survive minification. The
	// streaming export transform restores them after minification.
	if c.config.Minify {
		text = strings.ReplaceAll(text, " ", "\u00A0")
	}

	x := startCol * ColWidth
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
	xAttr := ""
	if x != 0 {
		xAttr = fmt.Sprintf(` x="%d"`, x)
	}
	classAttr := ""
	if len(classes) > 0 {
		classAttr = fmt.Sprintf(" class=%q", strings.Join(classes, " "))
	}

	fmt.Fprintf(w, `<text%s y="%d"%s>%s</text>`, xAttr, y, classAttr, svgTextEscaper.Replace(text))
}
