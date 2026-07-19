package anim

import (
	"fmt"
	"strings"

	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

// ProgressFlame maps a 0..1 fraction and stage label to a visual block.
func ProgressFlame(percent float64, stage, message string, frame int, s theme.Styles) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	flame := Flame(frame, percent, s)
	barW := 28
	filled := int(float64(barW) * percent)
	if filled > barW {
		filled = barW
	}
	bar := s.Fire.Render(strings.Repeat("█", filled)) + s.Muted.Render(strings.Repeat("░", barW-filled))
	stageLine := s.FireTip.Render(stage)
	if message != "" {
		stageLine += s.Muted.Render("  " + message)
	}
	pct := s.Subtitle.Render(fmt.Sprintf("  %d%%", int(percent*100)))
	return flame + "\n\n" + bar + pct + "\n" + stageLine
}
