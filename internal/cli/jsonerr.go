package cli

import (
	stderrors "errors"
	"strings"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
)

type jsonEmittedError struct{ err error }

func (e *jsonEmittedError) Error() string { return e.err.Error() }

func (e *jsonEmittedError) Unwrap() error { return e.err }

// jsonAlreadyEmitted wraps err to tell WrapJSONErrors that the command already
// wrote its own JSON envelope for this failure, so no second envelope is added.
func jsonAlreadyEmitted(err error) error { return &jsonEmittedError{err: err} }

// jsonAction returns the machine-stable action name for a failure envelope.
// It uses CommandPath with the root binary name stripped so nested commands stay
// unambiguous: "festival list" → "list", "festival marketplace list" →
// "marketplace list". Leaf Name() alone would collide those two.
func jsonAction(c *cobra.Command) string {
	path := strings.TrimSpace(c.CommandPath())
	if i := strings.IndexByte(path, ' '); i >= 0 {
		return path[i+1:]
	}
	if path != "" {
		return path
	}
	return c.Name()
}

// WrapJSONErrors makes every --json subcommand emit exactly one jsonout failure
// envelope on stdout when its RunE fails, carrying the machine error code, while
// still returning the error so the process exits non-zero. Commands without a
// --json flag and non --json invocations are left untouched, so the human path
// is unchanged. Apply it once to the assembled command tree.
func WrapJSONErrors(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		WrapJSONErrors(cmd)
		if cmd.RunE == nil || cmd.Flags().Lookup("json") == nil {
			continue
		}
		inner := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			err := inner(c, args)
			if err == nil {
				return nil
			}
			var emitted *jsonEmittedError
			if stderrors.As(err, &emitted) {
				return err
			}
			if f := c.Flags().Lookup("json"); f != nil && f.Value.String() == "true" {
				if writeErr := jsonout.Failure(c.OutOrStdout(), jsonAction(c), errpkg.Code(err), err.Error()); writeErr != nil {
					// Keep the domain error primary so exit codes/messages stay
					// useful; surface the write failure alongside it.
					return stderrors.Join(err, writeErr)
				}
			}
			return err
		}
	}
}
