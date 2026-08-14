package verify

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"

	canonical "github.com/gibson042/canonicaljson-go"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var ErrNonFiniteNumber = errpkg.New("E_CANONICAL_NONFINITE", "non-finite number cannot be canonically encoded")

var ErrTrailingJSON = errpkg.New("E_CANONICAL_TRAILING", "JSON document contains trailing data")

// CanonicalizeJSON decodes one JSON document without losing numeric precision
// and returns its RFC 8785 canonical encoding.
func CanonicalizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, errpkg.Wrap("E_CANONICAL_DECODE", err, "decode JSON document")
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = ErrTrailingJSON
		}
		return nil, errpkg.Wrap("E_CANONICAL_TRAILING", err, "reject trailing JSON data")
	}
	return Marshal(value)
}

// Marshal returns the RFC 8785 canonical encoding of v. Non-finite floats
// (NaN, +Inf, -Inf) anywhere in v cause ErrNonFiniteNumber.
func Marshal(v any) ([]byte, error) {
	if err := rejectNonFinite(reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	b, err := canonical.Marshal(v)
	if err != nil {
		return nil, errpkg.Wrap("E_CANONICAL_MARSHAL", err, "canonicalize value")
	}
	return b, nil
}

func rejectNonFinite(v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return ErrNonFiniteNumber
		}
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			return rejectNonFinite(v.Elem())
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := rejectNonFinite(iter.Value()); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := rejectNonFinite(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := rejectNonFinite(v.Field(i)); err != nil {
				return err
			}
		}
	}
	return nil
}
