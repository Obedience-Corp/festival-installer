package jsonout

import "io"

const SchemaVersion = "obey-installer/v1alpha1"

type Envelope struct {
	OK            bool        `json:"ok"`
	Action        string      `json:"action"`
	SchemaVersion string      `json:"schema_version"`
	Warnings      []string    `json:"warnings"`
	Data          any         `json:"data,omitempty"`
	Error         *ErrPayload `json:"error,omitempty"`
}

type ErrPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Success(w io.Writer, action string, data any, warnings []string) error {
	if warnings == nil {
		warnings = []string{}
	}
	return Print(w, Envelope{
		OK:            true,
		Action:        action,
		SchemaVersion: SchemaVersion,
		Warnings:      warnings,
		Data:          data,
	})
}

func Failure(w io.Writer, action, code, message string) error {
	return Print(w, Envelope{
		OK:            false,
		Action:        action,
		SchemaVersion: SchemaVersion,
		Warnings:      []string{},
		Error:         &ErrPayload{Code: code, Message: message},
	})
}
