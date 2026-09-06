package cli

import (
	"fmt"
	"io"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

// RenderError writes the user-facing rendering of a command failure. It is the
// single place the binary turns an error into terminal output, so an error that
// hides diagnostic detail hides it everywhere.
func RenderError(w io.Writer, err error) {
	if err == nil {
		return
	}
	_, _ = fmt.Fprintln(w, textsafe.Block(app.FriendlyMessage(err)))
}
