package svg

import (
	"bytes"
	"io"
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

type candidateWriter struct {
	w            io.Writer
	metrics      *CandidateMetrics
	collapseNBSP bool
	pendingC2    bool
	inDefs       bool
	pendingTag   []byte
	elementStack []int
	nextElement  int
	animatedAt   map[int]bool
}

func (w *candidateWriter) Write(p []byte) (int, error) {
	w.countBytes(p)
	w.countElements(p)
	return w.w.Write(p)
}

func (w *candidateWriter) countBytes(p []byte) {
	if !w.collapseNBSP {
		w.metrics.FinalBytes += int64(len(p))
		return
	}
	for _, value := range p {
		if w.pendingC2 {
			w.metrics.FinalBytes++
			w.pendingC2 = false
			if value == 0xa0 {
				continue
			}
		}
		if value == 0xc2 {
			w.pendingC2 = true
		} else {
			w.metrics.FinalBytes++
		}
	}
}

func (w *candidateWriter) finish() {
	if w.pendingC2 {
		w.metrics.FinalBytes++
		w.pendingC2 = false
	}
	w.metrics.AnimatedElements = len(w.animatedAt)
}

func (w *candidateWriter) countElements(p []byte) {
	if len(w.pendingTag) > 0 {
		p = append(w.pendingTag, p...)
		w.pendingTag = nil
	}
	for len(p) > 0 {
		start := bytes.IndexByte(p, '<')
		if start < 0 {
			return
		}
		p = p[start+1:]
		end := bytes.IndexByte(p, '>')
		if end < 0 {
			w.pendingTag = append(w.pendingTag[:0], '<')
			w.pendingTag = append(w.pendingTag, p...)
			return
		}
		token := bytes.TrimSpace(p[:end])
		p = p[end+1:]
		if len(token) == 0 || token[0] == '!' || token[0] == '?' {
			continue
		}
		if token[0] == '/' {
			w.elementStack = w.elementStack[:len(w.elementStack)-1]
			if string(bytes.TrimSpace(token[1:])) == "defs" {
				w.inDefs = false
			}
			continue
		}
		name := token
		if space := bytes.IndexAny(name, " \t\r\n/"); space >= 0 {
			name = name[:space]
		}
		nameString := string(name)
		w.countElement(nameString)
		if nameString == "animate" || nameString == "animateTransform" || nameString == "animateMotion" {
			if w.animatedAt == nil {
				w.animatedAt = map[int]bool{}
			}
			if len(w.elementStack) > 0 {
				w.animatedAt[w.elementStack[len(w.elementStack)-1]] = true
			}
		}
		if string(name) == "defs" {
			w.inDefs = true
		}
		if !bytes.HasSuffix(token, []byte{'/'}) {
			w.nextElement++
			w.elementStack = append(w.elementStack, w.nextElement)
		}
	}
}

func (w *candidateWriter) countElement(name string) {
	w.metrics.XMLNodes++
	if w.inDefs || name == "defs" {
		w.metrics.DefinitionNodes++
	} else {
		w.metrics.ActiveNodes++
	}
	switch name {
	case "text":
		w.metrics.TextNodes++
	case "rect":
		w.metrics.RectNodes++
	case "g":
		w.metrics.GroupNodes++
	case "use":
		w.metrics.UseNodes++
	case "animate", "animateTransform", "animateMotion":
		w.metrics.AnimationNodes++
	}
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
