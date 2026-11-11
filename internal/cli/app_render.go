package cli

import (
	"fmt"
	"io"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

// renderAppResult prints the human lines for an App install, update or uninstall.
func renderAppResult(w io.Writer, action string, res app.AppResult) error {
	loc, ver := textsafe.Line(res.Location), textsafe.Line(res.Version)
	var line string
	switch {
	case action == "uninstall" && res.Changed:
		line = fmt.Sprintf("Removed Festival App %s from %s", textsafe.Line(res.PreviousVersion), loc)
	case res.Changed && res.PreviousVersion != "" && res.PreviousVersion != res.Version:
		line = fmt.Sprintf("Updated Festival App %s -> %s at %s", textsafe.Line(res.PreviousVersion), ver, loc)
	case res.Changed:
		line = fmt.Sprintf("Installed Festival App %s at %s", ver, loc)
	case res.Version != "":
		line = fmt.Sprintf("Festival App is already current at %s", ver)
	}
	if line != "" {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	for _, note := range res.Notes {
		if _, err := fmt.Fprintln(w, textsafe.Block(note)); err != nil {
			return err
		}
	}
	return nil
}
