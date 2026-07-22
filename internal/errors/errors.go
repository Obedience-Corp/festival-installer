package errors

import (
	stderrors "errors"
	"fmt"
)

// UnknownCode is the machine code reported for errors that carry no *Error in
// their chain, so callers extracting a code always get a stable, non-empty
// value.
const UnknownCode = "E_UNKNOWN"

type Error struct {
	Code string
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

func New(code, msg string) error { return &Error{Code: code, Msg: msg} }

func Wrap(code string, err error, msg string) error {
	return &Error{Code: code, Msg: msg, Err: err}
}

// Code returns the machine code of the first *Error in err's chain, falling back
// to UnknownCode when err is nil or carries no coded error.
func Code(err error) string {
	var e *Error
	if stderrors.As(err, &e) {
		return e.Code
	}
	return UnknownCode
}
