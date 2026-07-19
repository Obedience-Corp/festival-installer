package anim

import (
	"fmt"
	"strings"

	"github.com/Obedience-Corp/obey-installer/internal/tui/theme"
)

// Booth is a side activity indicator on the home screen.
type Booth struct {
	Label  string
	Active bool
	Heat   int // 0..3 micro-activity level
}

var boothSpinners = []string{"·", "°", "*", "✦", "*"}

// RenderBooths draws concurrent side stages around the festival.
func RenderBooths(booths []Booth, tick int, s theme.Styles) string {
	if len(booths) == 0 {
		return ""
	}
	var parts []string
	for i, b := range booths {
		spin := boothSpinners[(tick+i*2)%len(boothSpinners)]
		heat := strings.Repeat("·", b.Heat+1)
		label := b.Label
		var line string
		if b.Active {
			line = s.Fire.Render(fmt.Sprintf("[%s %s %s]", spin, label, heat))
		} else {
			line = s.Muted.Render(fmt.Sprintf("[%s %s]", spin, label))
		}
		parts = append(parts, line)
	}
	// Two rows if many booths.
	if len(parts) <= 3 {
		return strings.Join(parts, "  ")
	}
	mid := (len(parts) + 1) / 2
	return strings.Join(parts[:mid], "  ") + "\n" + strings.Join(parts[mid:], "  ")
}

// DefaultHomeBooths returns the ambient multi-activity set.
func DefaultHomeBooths(activeIndex int) []Booth {
	labels := []string{"install", "browse", "market", "doctor", "path"}
	out := make([]Booth, len(labels))
	for i, l := range labels {
		out[i] = Booth{
			Label:  l,
			Active: i == activeIndex,
			Heat:   (i + activeIndex) % 3,
		}
	}
	return out
}
