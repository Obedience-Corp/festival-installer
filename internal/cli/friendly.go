package cli

import (
	stderrors "errors"
	"fmt"
	"io"

	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

// friendlyError is implemented by errors whose Error() carries diagnostic
// detail, such as raw git command output, that must never reach the terminal.
// internal/tui/components declares the same interface for the same reason; the
// CLI needs its own because the two render through different writers.
type friendlyError interface {
	Friendly() string
}

// friendlyMessage returns the text a user should see for err, preferring a
// Friendly() rendering when the error carries one.
func friendlyMessage(err error) string {
	if err == nil {
		return ""
	}
	var f friendlyError
	if stderrors.As(err, &f) {
		return f.Friendly()
	}
	return err.Error()
}

// RenderError writes the user-facing rendering of a command failure. It is the
// single place the binary turns an error into terminal output, so an error that
// hides diagnostic detail hides it everywhere.
func RenderError(w io.Writer, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintln(w, textsafe.Block(friendlyMessage(err)))
}
