package installer_test

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/installer"
)

func TestSatisfiesConstraint_SupportedOperators(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		constraint string
		want       bool
	}{
		{"gte equal", "0.4.0", ">=0.4.0", true},
		{"gte above", "0.4.5", ">=0.4.0", true},
		{"gte below", "0.3.0", ">=0.4.0", false},
		{"gt above", "0.4.1", ">0.4.0", true},
		{"gt equal", "0.4.0", ">0.4.0", false},
		{"lte equal", "0.4.0", "<=0.4.0", true},
		{"lte below", "0.3.9", "<=0.4.0", true},
		{"lte above", "0.4.1", "<=0.4.0", false},
		{"lt below", "0.3.9", "<0.4.0", true},
		{"lt equal", "0.4.0", "<0.4.0", false},
		{"eq match", "1.2.3", "=1.2.3", true},
		{"eq mismatch", "1.2.4", "=1.2.3", false},
		{"bare exact match", "1.2.3", "1.2.3", true},
		{"bare exact mismatch", "1.2.4", "1.2.3", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installer.SatisfiesConstraint(tc.version, tc.constraint)
			if err != nil {
				t.Fatalf("SatisfiesConstraint(%q, %q): unexpected error %v", tc.version, tc.constraint, err)
			}
			if got != tc.want {
				t.Fatalf("SatisfiesConstraint(%q, %q) = %v, want %v", tc.version, tc.constraint, got, tc.want)
			}
		})
	}
}

func TestSatisfiesConstraint_PrereleaseOrdering(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		constraint string
		want       bool
	}{
		{"rc satisfies gte rc", "1.2.0-rc.2", ">=1.2.0-rc.1", true},
		{"lower rc fails gte rc", "1.2.0-rc.1", ">=1.2.0-rc.2", false},
		{"final satisfies gte rc", "1.2.0", ">=1.2.0-rc.1", true},
		{"rc fails gte final", "1.2.0-rc.1", ">=1.2.0", false},
		{"rc less than final", "1.2.0-rc.1", "<1.2.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installer.SatisfiesConstraint(tc.version, tc.constraint)
			if err != nil {
				t.Fatalf("SatisfiesConstraint(%q, %q): unexpected error %v", tc.version, tc.constraint, err)
			}
			if got != tc.want {
				t.Fatalf("SatisfiesConstraint(%q, %q) = %v, want %v", tc.version, tc.constraint, got, tc.want)
			}
		})
	}
}

func TestSatisfiesConstraint_UnsupportedSyntaxErrors(t *testing.T) {
	cases := []struct {
		name       string
		constraint string
	}{
		{"compound range", ">=1.2.0 <2.0.0"},
		{"caret", "^1.2.0"},
		{"tilde", "~1.2.0"},
		{"minor wildcard", "1.2.*"},
		{"x range", "1.x"},
		{"two components", ">=1.2"},
		{"garbage core", "abc"},
		{"operator only", ">="},
		{"non-numeric operand", ">=abc"},
		{"empty", ""},
		{"space only", "   "},
		{"trailing prerelease dash", "1.2.0-"},
		{"four components", "1.2.3.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := installer.SatisfiesConstraint("1.2.3", tc.constraint)
			if err == nil {
				t.Fatalf("SatisfiesConstraint(1.2.3, %q): expected error, got nil", tc.constraint)
			}
			if !strings.Contains(err.Error(), "E_VERSION_CONSTRAINT") {
				t.Fatalf("SatisfiesConstraint(1.2.3, %q): expected E_VERSION_CONSTRAINT, got %v", tc.constraint, err)
			}
		})
	}
}

func TestSatisfiesConstraint_MalformedComparedVersionErrors(t *testing.T) {
	_, err := installer.SatisfiesConstraint("1.2.3.4", ">=1.0.0")
	if err == nil || !strings.Contains(err.Error(), "E_VERSION_CONSTRAINT") {
		t.Fatalf("expected E_VERSION_CONSTRAINT for malformed version, got %v", err)
	}
}
