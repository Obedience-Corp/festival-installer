package cli

import (
	stderrors "errors"

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
				_ = jsonout.Failure(c.OutOrStdout(), c.Name(), errpkg.Code(err), err.Error())
			}
			return err
		}
	}
}
