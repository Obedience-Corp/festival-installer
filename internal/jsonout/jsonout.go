package jsonout

import (
	"encoding/json"
	"io"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func Print(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return errpkg.Wrap("E_JSON_ENCODE", err, "encode json output")
	}
	return nil
}
