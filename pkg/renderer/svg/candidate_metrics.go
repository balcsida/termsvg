package svg

import (
	"github.com/mrmarble/termsvg/pkg/ir"
)

// CandidateMetrics describes the serialized size and structural cost of one SVG candidate.
type CandidateMetrics struct {
	FinalBytes         int64
	XMLNodes           int
	DefinitionNodes    int
	ActiveNodes        int
	TextNodes          int
	RectNodes          int
	GroupNodes         int
	UseNodes           int
	AnimationNodes     int
	AnimatedElements   int
	StateDefinitions   int
	MaxUseDepth        int
	MaxTranslatedWidth int64
	MaxTranslatedArea  int64
	LocalViewportCount int
	MaxViewportWidth   int
	MaxViewportHeight  int
}

func addPreparedMetrics(
	metrics *CandidateMetrics,
	content *preparedContent,
	options Options,
	contentWidth, contentHeight int,
) {
	metrics.StateDefinitions = len(content.frameStateIDs)
	for i := range content.bands {
		band := &content.bands[i]
		metrics.StateDefinitions += len(band.stateIDs)
		metrics.LocalViewportCount++
		metrics.MaxViewportWidth = max(metrics.MaxViewportWidth, band.width*ColWidth)
		metrics.MaxViewportHeight = max(metrics.MaxViewportHeight, band.height*RowHeight)
		if options.FrameSwitch == FrameSwitchTranslate && len(band.keyframes) > 1 {
			addTranslatedSurface(metrics, band.width*ColWidth, band.height*RowHeight, len(band.rows))
		}
	}
	if !options.usesLocalViewports() && options.FrameSwitch == FrameSwitchTranslate && len(content.frameKeyframes) > 1 {
		addTranslatedSurface(metrics, contentWidth, contentHeight, len(content.frameRows))
	}
	if len(content.rowDefs) > 0 {
		metrics.MaxUseDepth = 1
	}
	if metrics.StateDefinitions > 0 {
		metrics.MaxUseDepth++
	}
}

func addTranslatedSurface(metrics *CandidateMetrics, width, height, states int) {
	translatedWidth := int64(width * states)
	metrics.MaxTranslatedWidth = max(metrics.MaxTranslatedWidth, translatedWidth)
	metrics.MaxTranslatedArea = max(metrics.MaxTranslatedArea, translatedWidth*int64(height))
}

func addStructuralMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) {
	addMetric(metrics, false, "svg", 1)
	addMetric(metrics, false, "rect", 1)
	if c.config.ShowWindow {
		addMetric(metrics, false, "circle", len(c.config.Theme.WindowButtons))
	}
	addMetric(metrics, true, "defs", 1)
	addMetric(metrics, true, "clipPath", 1)
	addMetric(metrics, true, "rect", 1)
	addMetric(metrics, false, "g", 1)
	addMetric(metrics, false, "style", 1)

	for _, definition := range content.rowDefs {
		if c.rowElementCount(definition.row) > 1 {
			addMetric(metrics, true, "g", 1)
		}
		renderedDefinition := *definition
		renderedDefinition.id = ""
		addRowMetrics(metrics, c, true, &renderedDefinition)
	}
	for _, static := range c.plan.staticRows {
		addRowMetrics(metrics, c, false, &renderedRow{row: static})
	}

	for i := range content.frameStateIDs {
		addMetric(metrics, true, "g", 1)
		addStateRowsMetrics(metrics, c, true, content.frameRows[i:i+1])
	}
	for bandIndex := range content.bands {
		band := &content.bands[bandIndex]
		for i := range band.stateIDs {
			addMetric(metrics, true, "g", 1)
			addStateRowsMetrics(metrics, c, true, band.rows[i:i+1])
		}
	}

	animations := addActiveContentMetrics(metrics, c, content)
	if c.plan.cursorEverVisible {
		addMetric(metrics, false, "g", 1)
		addMetric(metrics, false, "rect", 1)
		if c.options.Animation == AnimationSMIL && len(c.cursorKeyframes()) > 1 {
			cursorAnimations := cursorAnimationCount(c.cursorKeyframes())
			addMetric(metrics, false, "animation", cursorAnimations)
			if cursorAnimations > 0 {
				animations++
			}
		}
	}
	metrics.AnimatedElements = animations
}

func addMetric(metrics *CandidateMetrics, definition bool, kind string, count int) {
	if count == 0 {
		return
	}
	metrics.XMLNodes += count
	if definition {
		metrics.DefinitionNodes += count
	} else {
		metrics.ActiveNodes += count
	}
	switch kind {
	case "text":
		metrics.TextNodes += count
	case "rect":
		metrics.RectNodes += count
	case "g":
		metrics.GroupNodes += count
	case "use":
		metrics.UseNodes += count
	case "animation":
		metrics.AnimationNodes += count
	}
}

func addRowMetrics(metrics *CandidateMetrics, c *canvas, definition bool, rendered *renderedRow) {
	if rendered.id != "" {
		addMetric(metrics, definition, "use", 1)
		return
	}
	addMetric(metrics, definition, "rect", len(c.backgroundSpans(rendered.row)))
	for _, run := range rendered.row.Runs {
		if shouldRenderText(run) {
			addMetric(metrics, definition, "text", 1)
		}
	}
}

func addStateRowsMetrics(
	metrics *CandidateMetrics,
	c *canvas,
	definition bool,
	states [][]*renderedRow,
) {
	for _, rows := range states {
		for _, rendered := range rows {
			addRowMetrics(metrics, c, definition, rendered)
		}
	}
}

func addActiveContentMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	switch {
	case c.options.usesLocalViewports():
		return addLocalViewportMetrics(metrics, c, content)
	case len(content.frameKeyframes) <= 1:
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return 0
	case c.options.FrameSwitch == FrameSwitchHref:
		addMetric(metrics, false, "use", 1)
		addMetric(metrics, false, "animation", 1)
		return 1
	default:
		addMetric(metrics, false, "g", 1+len(content.frameRows))
		animations := 0
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
			animations = 1
		}
		addStateRowsMetrics(metrics, c, false, content.frameRows)
		return animations
	}
}

func addLocalViewportMetrics(metrics *CandidateMetrics, c *canvas, content *preparedContent) int {
	animations := 0
	for i := range content.bands {
		band := &content.bands[i]
		addMetric(metrics, false, "svg", 1)
		if len(band.keyframes) <= 1 {
			addStateRowsMetrics(metrics, c, false, band.rows)
			continue
		}
		if c.options.FrameSwitch == FrameSwitchHref {
			addMetric(metrics, false, "use", 1)
			addMetric(metrics, false, "animation", 1)
			animations++
			continue
		}
		addMetric(metrics, false, "g", 1+len(band.rows))
		if c.options.Animation == AnimationSMIL {
			addMetric(metrics, false, "animation", 1)
			animations++
		}
		addStateRowsMetrics(metrics, c, false, band.rows)
	}
	return animations
}

func cursorAnimationCount(frames []keyframePoint[ir.Cursor]) int {
	position, visibility := false, false
	for i := 1; i < len(frames); i++ {
		position = position || frames[i-1].state.Col != frames[i].state.Col ||
			frames[i-1].state.Row != frames[i].state.Row
		visibility = visibility || frames[i-1].state.Visible != frames[i].state.Visible
	}
	count := 0
	if position {
		count++
	}
	if visibility {
		count++
	}
	return count
}
