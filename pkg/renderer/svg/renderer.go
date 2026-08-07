// Package svg provides an SVG renderer for terminal recordings.
// It generates animated SVGs using CSS keyframes or discrete SMIL timelines.
package svg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mrmarble/termsvg/internal/svgoutput"
	"github.com/mrmarble/termsvg/pkg/color"
	"github.com/mrmarble/termsvg/pkg/ir"
	"github.com/mrmarble/termsvg/pkg/renderer"
)

// Renderer implements the renderer.Renderer interface for SVG output.
type Renderer struct {
	config               renderer.Config
	options              Options
	onSemanticPlanBuild  func()
	onCandidateSerialize func()
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

type preparedCandidate struct {
	plan    *semanticPlan
	options Options
	content preparedContent
	metrics CandidateMetrics
	cost    preparedCandidateCost
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

func (r *Renderer) buildSemanticPlan(ctx context.Context, rec *ir.Recording) (semanticPlan, error) {
	if r.onSemanticPlanBuild != nil {
		r.onSemanticPlanBuild()
	}
	return buildSemanticPlan(ctx, rec, r.config.ShowCursor, r.options.MaxFPS, r.config.LoopCount)
}

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
	plan, err := r.buildSemanticPlan(ctx, rec)
	if err != nil {
		return err
	}
	candidate, err := r.prepareSelectedCandidate(ctx, rec, &plan)
	if err != nil {
		return err
	}
	return r.serializeCandidate(ctx, rec, w, candidate)
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
	plan, err := r.buildSemanticPlan(ctx, rec)
	if err != nil {
		return CandidateMetrics{}, err
	}
	candidate, err := r.prepareSelectedCandidate(ctx, rec, &plan)
	if err != nil {
		return CandidateMetrics{}, err
	}
	if candidate.metrics.FinalBytes == 0 {
		if err := r.measureCandidate(ctx, rec, candidate); err != nil {
			return CandidateMetrics{}, err
		}
	}
	return candidate.metrics, nil
}

func (r *Renderer) prepareSelectedCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
) (*preparedCandidate, error) {
	if r.options.Layout != LayoutAuto {
		return prepareCandidate(ctx, rec, plan, &r.config, r.options)
	}
	options := r.options
	options.Layout = LayoutFrames
	frames, err := prepareCandidate(ctx, rec, plan, &r.config, options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, frames); err != nil {
		return nil, err
	}
	options.Layout = LayoutBands
	bands, err := prepareCandidate(ctx, rec, plan, &r.config, options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, bands); err != nil {
		return nil, err
	}
	options.Layout = LayoutRegions
	regions, err := prepareCandidate(ctx, rec, plan, &r.config, options)
	if err != nil {
		return nil, err
	}
	if err := r.measureCandidate(ctx, rec, regions); err != nil {
		return nil, err
	}
	selected := selectPreparedCandidate(r.options.AutoObjective, frames, bands, regions)
	r.logAutoCandidates(selected, frames, bands, regions)
	return selected, nil
}

func selectPreparedCandidate(objective AutoObjective, candidates ...*preparedCandidate) *preparedCandidate {
	selected := candidates[0]
	for _, candidate := range candidates[1:] {
		if objective == AutoObjectiveRuntime && runtimeCandidateLess(&candidate.metrics, &selected.metrics) ||
			objective != AutoObjectiveRuntime && candidate.metrics.FinalBytes < selected.metrics.FinalBytes {
			selected = candidate
		}
	}
	return selected
}

func runtimeCandidateLess(left, right *CandidateMetrics) bool {
	leftViewportArea := int64(left.MaxViewportWidth) * int64(left.MaxViewportHeight)
	rightViewportArea := int64(right.MaxViewportWidth) * int64(right.MaxViewportHeight)
	comparisons := [...]struct{ left, right uint64 }{
		{left.PeakLiveNodeEstimate, right.PeakLiveNodeEstimate},
		{left.DurationWeightedInstantiatedNodeNanos, right.DurationWeightedInstantiatedNodeNanos},
		{left.UseTargetSwitches, right.UseTargetSwitches},
	}
	for _, comparison := range comparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	integerComparisons := [...]struct{ left, right int64 }{
		{int64(left.AnimationNodes), int64(right.AnimationNodes)},
		{int64(left.AnimatedElements), int64(right.AnimatedElements)},
		{int64(left.LocalViewportCount), int64(right.LocalViewportCount)},
		{leftViewportArea, rightViewportArea},
		{left.MaxTranslatedArea, right.MaxTranslatedArea},
		{left.FinalBytes, right.FinalBytes},
	}
	for _, comparison := range integerComparisons {
		if comparison.left != comparison.right {
			return comparison.left < comparison.right
		}
	}
	return false
}

func (r *Renderer) logAutoCandidates(selected *preparedCandidate, candidates ...*preparedCandidate) {
	if !r.config.Debug {
		return
	}
	for _, candidate := range candidates {
		metrics := candidate.metrics
		log.Printf("[SVG] auto candidate layout=%s bytes=%d source_nodes=%d definition_nodes=%d "+
			"peak_instantiated_nodes=%d animation_nodes=%d viewport_count=%d translated_area=%d "+
			"objective=%s selected=%s",
			candidate.options.Layout, metrics.FinalBytes, metrics.SourceActiveNodes, metrics.SourceDefinitionNodes,
			metrics.PeakInstantiatedNodes, metrics.AnimationNodes, metrics.LocalViewportCount, metrics.MaxTranslatedArea,
			r.options.AutoObjective, selected.options.Layout)
	}
}

func prepareCandidate(
	ctx context.Context,
	rec *ir.Recording,
	plan *semanticPlan,
	config *renderer.Config,
	options Options,
) (*preparedCandidate, error) {
	c := &canvas{
		rec:        rec,
		plan:       *plan,
		config:     *config,
		options:    options,
		classNames: rec.Colors.GenerateClassNames(),
	}
	content, err := c.prepareContentContext(ctx)
	if err != nil {
		return nil, err
	}
	candidate := &preparedCandidate{plan: plan, options: options, content: content}
	addPreparedMetrics(&candidate.metrics, &content, options, c.contentWidth(), c.contentHeight())
	if err := addStructuralMetrics(&candidate.metrics, c, &content); err != nil {
		return nil, err
	}
	candidate.cost = buildPreparedCandidateCost(c, candidate)
	return candidate, nil
}

func (r *Renderer) measureCandidate(ctx context.Context, rec *ir.Recording, candidate *preparedCandidate) error {
	bytes, err := costPreparedCandidate(ctx, rec, &r.config, candidate)
	if err != nil {
		return err
	}
	candidate.metrics.FinalBytes = bytes
	return nil
}

func writeFinalSVG(w io.Writer, minify bool, render func(io.Writer) error) error {
	if minify {
		return svgoutput.Write(w, render)
	}
	return render(w)
}

func (r *Renderer) serializeCandidate(
	ctx context.Context,
	rec *ir.Recording,
	w io.Writer,
	candidate *preparedCandidate,
) error {
	if r.onCandidateSerialize != nil {
		r.onCandidateSerialize()
	}
	buf := bufio.NewWriterSize(w, 64*1024)
	c := &canvas{
		w: buf, rec: rec, plan: *candidate.plan, config: r.config, options: candidate.options,
		classNames: rec.Colors.GenerateClassNames(), metrics: &candidate.metrics,
	}
	if err := c.render(ctx, &candidate.content); err != nil {
		return err
	}
	return buf.Flush()
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
	if c.plan.width == 0 {
		return c.rec.Width * ColWidth
	}
	return c.plan.width * ColWidth
}

func (c *canvas) contentHeight() int {
	if c.plan.height == 0 {
		return c.rec.Height * RowHeight
	}
	return c.plan.height * RowHeight
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

func (c *canvas) render(ctx context.Context, content *preparedContent) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.writeSVGOpen()

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

	c.writeDefs(content)
	c.writeContentGroupOpen(contentY)

	c.writeStyles(content)
	for _, row := range c.plan.staticRows {
		c.writeRow(c.w, row)
	}
	c.writeContent(content)
	c.writeCursor()

	fmt.Fprint(c.w, `</g></svg>`)

	return nil
}

func (c *canvas) writeSVGOpen() {
	xmlSpace := ` xml:space="preserve"`
	if c.config.Minify {
		xmlSpace = ""
	}
	fmt.Fprintf(c.w, `<svg xmlns="http://www.w3.org/2000/svg"%s width="%s" height="%s">`,
		xmlSpace, c.xmlInt(c.paddedWidth()), c.xmlInt(c.paddedHeight()))
}

func (c *canvas) writeDefs(content *preparedContent) {
	fmt.Fprintf(c.w, `<defs><clipPath id="clip"><rect width="%s" height="%s"/></clipPath>`,
		c.xmlInt(c.contentWidth()), c.xmlInt(c.contentHeight()))
	c.writeRowDefs(content.rowDefs)
	c.writeStateDefs(content)
	fmt.Fprint(c.w, `</defs>`)
}

func (c *canvas) writeContentGroupOpen(contentY int) {
	fmt.Fprintf(c.w, `<g transform="translate(%s,%s)" clip-path="url(#clip)">`, c.xmlInt(Padding), c.xmlInt(contentY))
}

func (c *canvas) writeBackground() {
	bgHex := color.RGBAtoHex(c.config.Theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect width="100%%" height="100%%" fill="%s"/>`, bgHex)
}

func (c *canvas) writeWindow() {
	theme := c.config.Theme

	// Window background with rounded corners
	bgHex := color.RGBAtoHex(theme.WindowBackground)
	fmt.Fprintf(c.w, `<rect rx="%s" width="100%%" height="100%%" fill="%s"/>`, c.xmlInt(windowCornerRadius), bgHex)

	// Window buttons (close, minimize, maximize)
	buttonY := Padding
	for i, btnColor := range theme.WindowButtons {
		btnHex := color.RGBAtoHex(btnColor)
		x := Padding + i*windowButtonSpacing
		fmt.Fprintf(c.w, `<circle cx="%s" cy="%s" r="%s" fill="%s"/>`, c.xmlInt(x), c.xmlInt(buttonY), c.xmlInt(windowButtonRadius), btnHex)
	}
}

func (c *canvas) writeStyles(content *preparedContent) {
	var sb strings.Builder
	sb.WriteString("<style>")

	if c.options.Animation == AnimationCSS {
		if c.options.usesLocalViewports() {
			written := make(map[string]bool)
			for i := range content.bands {
				band := &content.bands[i]
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
		if c.config.Minify && !renderedRowsHaveOutput(content.frameRows[i]) {
			fmt.Fprintf(c.w, `<g id="%s"/>`, id)
			continue
		}
		fmt.Fprintf(c.w, `<g id="%s">`, id)
		c.writeFrameRows(content.frameRows[i])
		fmt.Fprint(c.w, `</g>`)
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		for i, id := range band.stateIDs {
			if c.config.Minify && !renderedRowsHaveOutput(band.rows[i]) {
				fmt.Fprintf(c.w, `<g id="%s"/>`, id)
				continue
			}
			fmt.Fprintf(c.w, `<g id="%s">`, id)
			c.writeFrameRows(band.rows[i])
			fmt.Fprint(c.w, `</g>`)
		}
	}
}

func (c *canvas) writeContent(content *preparedContent) {
	if c.options.usesLocalViewports() {
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
	for bandIndex := range bands {
		c.writeBand(&bands[bandIndex])
	}
}

func (c *canvas) writeBand(band *preparedBand) {
	width := band.width * ColWidth
	height := band.height * RowHeight
	x, y := band.x*ColWidth, band.y*RowHeight
	xAttr, yAttr := fmt.Sprintf(` x="%s"`, c.xmlInt(x)), fmt.Sprintf(` y="%s"`, c.xmlInt(y))
	if c.config.Minify && x == 0 {
		xAttr = ""
	}
	if c.config.Minify && y == 0 {
		yAttr = ""
	}
	fmt.Fprintf(c.w, `<svg%s%s width="%s" height="%s" overflow="hidden">`,
		xAttr, yAttr, c.xmlInt(width), c.xmlInt(height))
	if len(band.keyframes) <= 1 {
		if len(band.rows) > 0 {
			c.writeFrameRows(band.rows[len(band.rows)-1])
		}
		fmt.Fprint(c.w, `</svg>`)
		return
	}
	if c.options.FrameSwitch == FrameSwitchHref {
		c.writeHrefSequence(band.keyframes, band.stateIDs)
		fmt.Fprint(c.w, `</svg>`)
		return
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
		if c.config.Minify && !renderedRowsHaveOutput(rows) {
			fmt.Fprintf(c.w, `<g transform="translate(%s)"/>`, c.xmlInt(width*i))
			continue
		}
		fmt.Fprintf(c.w, `<g transform="translate(%s)">`, c.xmlInt(width*i))
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

func renderedRowsHaveOutput(rows []*renderedRow) bool {
	for _, row := range rows {
		if row.id != "" || row.svg != "" {
			return true
		}
	}
	return false
}

func (c *canvas) writeRow(w io.Writer, row ir.Row) {
	for _, span := range c.backgroundSpans(row) {
		x := span.startCol * ColWidth
		xAttr := ""
		if x != 0 {
			xAttr = fmt.Sprintf(` x="%s"`, c.xmlInt(x))
		}
		fmt.Fprintf(w, `<rect class="%s"%s y="%s" width="%s" height="%s"/>`,
			c.classNames[span.colorID], xAttr, c.xmlInt(row.Y*RowHeight),
			c.xmlInt((span.endCol-span.startCol)*ColWidth), c.xmlInt(RowHeight))
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
	fmt.Fprintf(c.w, `<g transform="translate(%s,%s)" visibility="%s"%s>`,
		c.xmlInt(point.state.Col*ColWidth), c.xmlInt(point.state.Row*RowHeight), cursorVisibility(point.state), style)
	if len(frames) > 1 && c.options.Animation == AnimationSMIL {
		c.writeSMILCursor(c.w, frames)
	}
	fmt.Fprintf(c.w, `<rect class="cursor" width="%s" height="%s"/></g>`, c.xmlInt(ColWidth), c.xmlInt(RowHeight))
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
		xAttr = fmt.Sprintf(` x="%s"`, c.xmlInt(x))
	}
	classAttr := ""
	if len(classes) > 0 {
		classAttr = fmt.Sprintf(" class=%q", strings.Join(classes, " "))
	}

	fmt.Fprintf(w, `<text%s y="%s"%s>%s</text>`, xAttr, c.xmlInt(y), classAttr, svgTextEscaper.Replace(text))
}

func (c *canvas) xmlInt(value int) string {
	plain := strconv.Itoa(value)
	if !c.config.Minify || value == 0 {
		return plain
	}
	sign, digits := "", plain
	if digits[0] == '-' {
		sign, digits = "-", digits[1:]
	}
	zeros := len(digits) - len(strings.TrimRight(digits, "0"))
	if zeros == 0 {
		return plain
	}
	scientific := sign + strings.TrimRight(digits, "0") + "e" + strconv.Itoa(zeros)
	if len(scientific) < len(plain) {
		return scientific
	}
	return plain
}
