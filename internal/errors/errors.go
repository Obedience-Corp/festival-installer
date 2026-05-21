package errors

import "fmt"

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
