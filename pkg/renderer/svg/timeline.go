package svg

import (
	"fmt"
	"strings"

	"github.com/mrmarble/termsvg/pkg/ir"
)

func (c *canvas) loopCount() string {
	if c.config.LoopCount == -1 {
		return "1"
	}
	if c.config.LoopCount > 0 {
		return fmt.Sprintf("%d", c.config.LoopCount)
	}
	return "infinite"
}

func (c *canvas) generateCursorKeyframes() string {
	var sb strings.Builder
	sb.WriteString("@keyframes cursor{")
	for _, point := range c.plan.cursor.points {
		fmt.Fprintf(&sb, "%.3f%%{transform:translate(%dpx,%dpx);visibility:%s}",
			point.time.Seconds()/c.plan.duration.Seconds()*100,
			point.cursor.Col*ColWidth, point.cursor.Row*RowHeight, cursorVisibility(point.cursor))
	}
	sb.WriteString("}")
	return sb.String()
}

func cursorVisibility(cursor ir.Cursor) string {
	if cursor.Visible {
		return "visible"
	}
	return "hidden"
}
