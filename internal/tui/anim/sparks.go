package anim

import (
	"math"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/tui/theme"
)

// SparkField drifts small embers for multi-activity energy.
type SparkField struct {
	Width  int
	Height int
	Tick   int
}

// Render draws a field of drifting sparks.
func (f SparkField) Render(s theme.Styles) string {
	if f.Width < 4 || f.Height < 1 {
		return ""
	}
	var lines []string
	for y := 0; y < f.Height; y++ {
		row := make([]rune, f.Width)
		for x := range row {
			row[x] = ' '
		}
		// Place a few sparks per row with phase offset.
		for i := 0; i < 3; i++ {
			phase := f.Tick*2 + y*7 + i*13
			x := int(math.Abs(float64(phase*3+y))) % f.Width
			glyphs := []rune{'.', '·', '*', '°'}
			g := glyphs[(phase+i)%len(glyphs)]
			row[x] = g
		}
		line := string(row)
		// Style alternating heat.
		if (y+f.Tick)%2 == 0 {
			lines = append(lines, s.FireTip.Render(line))
		} else {
			lines = append(lines, s.FireCore.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

// Celebrate renders a short spark burst line.
func Celebrate(frame int, s theme.Styles) string {
	patterns := []string{
		"  *  .  *  .  *  ",
		" .  *  °  *  .  ",
		"*  °  *  .  *  °",
		"  .  *  .  *  . ",
	}
	p := patterns[frame%len(patterns)]
	return s.FireTip.Render(p) + "\n" + s.OK.Render("  ready: the fire is lit")
}
