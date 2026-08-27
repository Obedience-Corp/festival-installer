package launch

import (
	"context"
	"os"
)

// ReplaceSelf re-execs the festival hub at the resolved path with the same
// arguments this process started with. On success it does not return.
func ReplaceSelf(ctx context.Context) error {
	path, err := Resolve(ctx, "festival")
	if err != nil {
		return err
	}
	argv := append([]string{path}, os.Args[1:]...)
	return replaceExec(path, argv, os.Environ())
}
