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
	for _, band := range content.bands {
		metrics.StateDefinitions += len(band.stateIDs)
		metrics.LocalViewportCount++
		metrics.MaxViewportWidth = max(metrics.MaxViewportWidth, band.width*ColWidth)
		metrics.MaxViewportHeight = max(metrics.MaxViewportHeight, band.height*RowHeight)
		if options.FrameSwitch == FrameSwitchTranslate && len(band.keyframes) > 1 {
			addTranslatedSurface(metrics, band.width*ColWidth, band.height*RowHeight, len(band.rows))
		}
	}
	if options.Layout != LayoutBands && options.FrameSwitch == FrameSwitchTranslate && len(content.frameKeyframes) > 1 {
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
	add := func(definition bool, kind string, count int) {
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
	add(false, "svg", 1)
	add(false, "rect", 1)
	if c.config.ShowWindow {
		add(false, "circle", len(c.config.Theme.WindowButtons))
	}
	add(true, "defs", 1)
	add(true, "clipPath", 1)
	add(true, "rect", 1)
	add(false, "g", 1)
	add(false, "style", 1)

	row := func(definition bool, rendered *renderedRow) {
		if rendered.id != "" {
			add(definition, "use", 1)
			return
		}
		add(definition, "rect", len(c.backgroundSpans(rendered.row)))
		for _, run := range rendered.row.Runs {
			if shouldRenderText(run) {
				add(definition, "text", 1)
			}
		}
	}
	for _, definition := range content.rowDefs {
		if c.rowElementCount(definition.row) > 1 {
			add(true, "g", 1)
		}
		copy := *definition
		copy.id = ""
		row(true, &copy)
	}
	for _, static := range c.plan.staticRows {
		row(false, &renderedRow{row: static})
	}

	stateRows := func(definition bool, states [][]*renderedRow) {
		for _, rows := range states {
			for _, rendered := range rows {
				row(definition, rendered)
			}
		}
	}
	for i := range content.frameStateIDs {
		add(true, "g", 1)
		stateRows(true, content.frameRows[i:i+1])
	}
	for _, band := range content.bands {
		for i := range band.stateIDs {
			add(true, "g", 1)
			stateRows(true, band.rows[i:i+1])
		}
	}

	animations := 0
	if c.options.Layout == LayoutBands {
		for _, band := range content.bands {
			add(false, "svg", 1)
			if len(band.keyframes) <= 1 {
				stateRows(false, band.rows)
				continue
			}
			if c.options.FrameSwitch == FrameSwitchHref {
				add(false, "use", 1)
				add(false, "animation", 1)
				animations++
				continue
			}
			add(false, "g", 1+len(band.rows))
			if c.options.Animation == AnimationSMIL {
				add(false, "animation", 1)
				animations++
			}
			stateRows(false, band.rows)
		}
	} else if len(content.frameKeyframes) <= 1 {
		stateRows(false, content.frameRows)
	} else if c.options.FrameSwitch == FrameSwitchHref {
		add(false, "use", 1)
		add(false, "animation", 1)
		animations++
	} else {
		add(false, "g", 1+len(content.frameRows))
		if c.options.Animation == AnimationSMIL {
			add(false, "animation", 1)
			animations++
		}
		stateRows(false, content.frameRows)
	}
	if c.plan.cursorEverVisible {
		add(false, "g", 1)
		add(false, "rect", 1)
		if c.options.Animation == AnimationSMIL && len(c.cursorKeyframes()) > 1 {
			cursorAnimations := cursorAnimationCount(c.cursorKeyframes())
			add(false, "animation", cursorAnimations)
			if cursorAnimations > 0 {
				animations++
			}
		}
	}
	metrics.AnimatedElements = animations
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
