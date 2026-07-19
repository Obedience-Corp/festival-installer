package anim

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

// Flame frames (block height animation). Index cycles for breath.
var flameFrames = [][]string{
	{
		"    .  ",
		"   )(  ",
		"  )##( ",
		" )####(",
		")######(",
		" `####' ",
	},
	{
		"   .   ",
		"   )(  ",
		"  )##( ",
		" )####(",
		")######(",
		" `####' ",
	},
	{
		"  . .  ",
		"  )#(  ",
		" )###( ",
		")#####(",
		")######(",
		" `####' ",
	},
	{
		"   .*  ",
		"  )##( ",
		" )####(",
		")######(",
		")#######(",
		" `#####' ",
	},
	{
		"  * .  ",
		"  )#(  ",
		" )###( ",
		")#####(",
		")######(",
		" `####' ",
	},
	{
		"   .   ",
		"  )#(  ",
		" )###( ",
		")#####(",
		")######(",
		" `####' ",
	},
}

// SmallFlame is a compact brand chip for reduced motion / tight headers.
func SmallFlame(s theme.Styles) string {
	return s.FireTip.Render(")^(") + "\n" + s.Fire.Render(")#(")
}

// Flame renders a breathing flame at frame index.
// heightScale 0..1 grows the flame (progress); 1 is full ambient height.
func Flame(frame int, heightScale float64, s theme.Styles) string {
	if heightScale < 0 {
		heightScale = 0
	}
	if heightScale > 1 {
		heightScale = 1
	}
	if frame < 0 {
		frame = 0
	}
	f := flameFrames[frame%len(flameFrames)]
	// Scale: show bottom N lines proportional to heightScale (min 2).
	n := 2 + int(float64(len(f)-2)*heightScale)
	if n > len(f) {
		n = len(f)
	}
	start := len(f) - n
	var b strings.Builder
	for i := start; i < len(f); i++ {
		line := f[i]
		var styled string
		rel := float64(i-start) / float64(max(n-1, 1))
		switch {
		case rel < 0.35:
			styled = s.FireTip.Render(line)
		case rel < 0.7:
			styled = s.Fire.Render(line)
		default:
			styled = s.FireCore.Render(line)
		}
		b.WriteString(styled)
		if i < len(f)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// StaticFlame is a non-animated full flame for reduced motion.
func StaticFlame(s theme.Styles) string {
	return Flame(0, 1, s)
}

// Wordmark renders FESTIVAL in fire colors.
func Wordmark(s theme.Styles) string {
	return s.Fire.Render("FESTIVAL")
}

// Tagline is the product metaphor line.
const Tagline = "a festival has a lot of different things going on, just like you"

// BootView composes splash content.
func BootView(frame int, width int, s theme.Styles, reduced bool) string {
	var body string
	if reduced {
		body = StaticFlame(s)
	} else {
		body = Flame(frame, 1, s)
	}
	wm := Wordmark(s)
	tag := s.Tagline.Render(Tagline)
	block := lipgloss.JoinVertical(lipgloss.Center, body, "", wm, "", tag)
	if width < 20 {
		return block
	}
	return lipgloss.Place(width, 14, lipgloss.Center, lipgloss.Center, block)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
