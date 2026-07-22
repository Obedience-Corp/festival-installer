package installer

import (
	"strconv"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

// constraintOps are the comparison operators the version-constraint grammar
// accepts, ordered longest-first so prefix matching is unambiguous.
var constraintOps = []string{">=", "<=", ">", "<", "="}

// SatisfiesConstraint reports whether version satisfies constraint. The
// supported grammar is a single comparison operator (>=, >, <=, <, =, or none
// meaning exact match) followed by one strict MAJOR.MINOR.PATCH version with an
// optional prerelease or build suffix. Whitespace around the whole constraint
// and between the operator and version is accepted at runtime (e.g. ">= 0.4.0")
// so hand-authored host/runtime constraints stay ergonomic; marketplace JSON
// schema may still require the compact form without spaces.
//
// Only a single suffix is accepted: either prerelease ("1.2.3-rc.1") or build
// metadata ("1.2.3+build"), not both. That matches the manifest schema pattern
// and keeps ordering via lessSemver well-defined for this grammar.
//
// Anything outside that grammar (compound ranges, caret/tilde operators,
// wildcards, or non-numeric core parts) is a hard error rather than a silent
// partial match, and a malformed compared version is reported instead of being
// degraded to zeroed components.
func SatisfiesConstraint(version, constraint string) (bool, error) {
	op, want, err := parseConstraint(constraint)
	if err != nil {
		return false, err
	}
	v := strings.TrimSpace(version)
	if _, _, err := parseStrictVersion(v); err != nil {
		return false, errpkg.Wrap("E_VERSION_CONSTRAINT", err, "version "+version)
	}
	switch op {
	case ">=":
		return !lessSemver(v, want), nil
	case ">":
		return lessSemver(want, v), nil
	case "<=":
		return !lessSemver(want, v), nil
	case "<":
		return lessSemver(v, want), nil
	default:
		return !lessSemver(v, want) && !lessSemver(want, v), nil
	}
}

// parseConstraint splits a constraint into its comparison operator and version
// operand, rejecting any input outside the supported grammar. The returned
// version is a validated strict MAJOR.MINOR.PATCH string suitable for ordering
// via lessSemver. Operands are trimmed so ">= 1.2.3" is accepted.
func parseConstraint(constraint string) (op, version string, err error) {
	trimmed := strings.TrimSpace(constraint)
	if trimmed == "" {
		return "", "", errpkg.New("E_VERSION_CONSTRAINT", "empty version constraint")
	}
	op, rest := splitConstraintOp(trimmed)
	rest = strings.TrimSpace(rest)
	if _, _, perr := parseStrictVersion(rest); perr != nil {
		return "", "", errpkg.Wrap("E_VERSION_CONSTRAINT", perr, "constraint "+constraint)
	}
	return op, rest, nil
}

func splitConstraintOp(c string) (op, rest string) {
	for _, o := range constraintOps {
		if strings.HasPrefix(c, o) {
			return o, c[len(o):]
		}
	}
	return "", c
}

// parseStrictVersion parses a strict MAJOR.MINOR.PATCH version with an optional
// prerelease or build suffix, matching the release version grammar. Unlike the
// ordering parser used for schema-validated release versions, it surfaces a
// non-numeric core component as an error rather than treating it as zero, so
// callers on the constraint path never compare against a degraded version.
//
// Only one of prerelease (-) or build (+) is accepted: the first of those
// characters splits the core, and the remainder must be a valid ident without
// the other marker (so "1.2.3-rc.1+build" is rejected). This is intentional and
// matches the manifest schema's single-suffix pattern.
func parseStrictVersion(v string) ([3]int, string, error) {
	var core [3]int
	if v == "" {
		return core, "", errpkg.New("E_VERSION_PARSE", "empty version")
	}
	body := v
	ident := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		body = v[:i]
		ident = v[i+1:]
		if ident == "" || !isVersionIdent(ident) {
			kind := "prerelease"
			if v[i] == '+' {
				kind = "build metadata"
			}
			return core, "", errpkg.New("E_VERSION_PARSE", "version "+v+" has invalid "+kind+" "+v[i:])
		}
	}
	parts := strings.Split(body, ".")
	if len(parts) != 3 {
		return core, "", errpkg.New("E_VERSION_PARSE", "version "+v+" is not MAJOR.MINOR.PATCH")
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return core, "", errpkg.Wrap("E_VERSION_PARSE", err, "version "+v+" has non-numeric component "+p)
		}
		core[i] = n
	}
	return core, ident, nil
}

func isVersionIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c == '.', c == '-':
		default:
			return false
		}
	}
	return true
}
