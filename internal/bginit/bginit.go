// Package bginit seeds lipgloss's background-color cache before bubbletea can
// issue OSC/DSR terminal queries that hang on bare PTYs and CI.
//
// Mirrors projects/camp/internal/bginit. Import from cmd/festival with a blank
// import before the tui package runs.
package bginit

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func init() {
	lipgloss.SetHasDarkBackground(backgroundIsDark(os.Getenv("COLORFGBG")))
}

func backgroundIsDark(colorFGBG string) bool {
	if !strings.Contains(colorFGBG, ";") {
		return true
	}
	fields := strings.Split(colorFGBG, ";")
	bg, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return true
	}
	return bg >= 0 && bg <= 8 && bg != 7
}
