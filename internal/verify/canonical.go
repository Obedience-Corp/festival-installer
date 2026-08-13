package verify

import (
	"math"
	"reflect"

	canonical "github.com/gibson042/canonicaljson-go"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var ErrNonFiniteNumber = errpkg.New("E_CANONICAL_NONFINITE", "non-finite number cannot be canonically encoded")

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
