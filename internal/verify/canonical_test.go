package verify_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/verify"
)

func TestMarshal_GoldenFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "canonical")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			inPath := filepath.Join(root, e.Name(), "input.json")
			expPath := filepath.Join(root, e.Name(), "expected.canonical.json")

			raw, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatalf("decode input: %v", err)
			}

			got, err := verify.Marshal(v)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			want, err := os.ReadFile(expPath)
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			want = bytes.TrimRight(want, "\n")
			if !bytes.Equal(got, want) {
				t.Fatalf("byte mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
	if count < 6 {
		t.Fatalf("expected >=6 fixture dirs, got %d", count)
	}
}

func TestMarshal_Deterministic(t *testing.T) {
	v := map[string]any{
		"z": []any{3.0, 1.0, 2.0},
		"a": map[string]any{"x": "y", "p": "q"},
		"m": "hello",
		"n": float64(42),
	}

	first, err := verify.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 1000; i++ {
		got, err := verify.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal iter %d: %v", i, err)
		}
		if !bytes.Equal(got, first) {
			t.Fatalf("non-deterministic at iter %d\n got: %s\nwant: %s", i, got, first)
		}
	}
}

func TestMarshal_RejectsNonFinite(t *testing.T) {
	cases := []struct {
		name string
		v    any
	}{
		{"NaN_root", math.NaN()},
		{"PosInf_root", math.Inf(+1)},
		{"NegInf_root", math.Inf(-1)},
		{"NaN_nested", map[string]any{"x": []any{1.0, math.NaN()}}},
		{"Inf_in_struct", struct{ F float64 }{F: math.Inf(+1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verify.Marshal(tc.v)
			if !errors.Is(err, verify.ErrNonFiniteNumber) {
				t.Fatalf("expected ErrNonFiniteNumber, got %v", err)
			}
		})
	}
}

func TestMarshal_RejectsNonFiniteBehindPointer(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(+1)
	cases := []struct {
		name string
		v    any
	}{
		{"NaN_pointer_field", struct{ V *float64 }{V: &nan}},
		{"Inf_pointer_in_map", map[string]any{"p": &inf}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verify.Marshal(tc.v); !errors.Is(err, verify.ErrNonFiniteNumber) {
				t.Fatalf("expected ErrNonFiniteNumber for non-finite behind a *T pointer, got %v", err)
			}
		})
	}
}

func BenchmarkMarshal10K(b *testing.B) {
	// Build a synthetic ~10KB document of nested map/array.
	v := map[string]any{}
	for i := 0; i < 200; i++ {
		v[strings.Repeat("k", 5)+itoa(i)] = []any{i, "value-" + itoa(i), float64(i) * 1.5}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := verify.Marshal(v); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	pos := len(b)
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}
