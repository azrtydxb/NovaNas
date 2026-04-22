package components

import (
	"fmt"
	"strings"
)

// Progress is a minimal ASCII progress bar.
type Progress struct {
	Width   int
	Percent float64 // 0.0 - 1.0
	Label   string
}

// View renders the bar.
func (p Progress) View() string {
	w := p.Width
	if w <= 0 {
		w = 40
	}
	filled := int(float64(w) * p.Percent)
	if filled > w {
		filled = w
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", w-filled)
	return fmt.Sprintf("[%s] %3.0f%%  %s", bar, p.Percent*100, p.Label)
}
