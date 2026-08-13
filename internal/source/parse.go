package source

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

const manifestFilename = "obey-marketplace.json"

var (
	ErrManifestInvalid = errpkg.New("E_MANIFEST_INVALID", "marketplace manifest failed schema validation")
	ErrNotAMarketplace = errpkg.New("E_NOT_A_MARKETPLACE", "no obey-marketplace.json at repo root")
)

type ManifestError struct {
	field   string
	message string
}

func (e *ManifestError) Error() string {
	if e.field == "" {
		return "manifest invalid: " + e.message
	}
	return "manifest invalid at " + e.field + ": " + e.message
}

func (e *ManifestError) Field() string { return e.field }

func (e *ManifestError) Is(target error) bool {
	return target == ErrManifestInvalid
}

var (
	compileOnce sync.Once
	compiled    *jsonschema.Schema
	compileErr  error
)

func ensureSchema() error {
	compileOnce.Do(func() {
		data, err := schemaFS.ReadFile("schemas/marketplace.schema.json")
		if err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "read marketplace schema")
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("marketplace.json", strings.NewReader(string(data))); err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "add marketplace schema")
			return
		}
		sch, err := compiler.Compile("marketplace.json")
		if err != nil {
			compileErr = errpkg.Wrap("E_SCHEMA_COMPILE", err, "compile marketplace schema")
			return
		}
		compiled = sch
	})
	return compileErr
}

func ParseMarketplace(ctx context.Context, raw []byte) (Marketplace, error) {
	if err := ctx.Err(); err != nil {
		return Marketplace{}, errpkg.Wrap("E_PARSE_CTX", err, "context cancelled")
	}
	if err := ensureSchema(); err != nil {
		return Marketplace{}, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Marketplace{}, &ManifestError{message: "invalid JSON: " + err.Error()}
	}
	if err := compiled.Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			return Marketplace{}, &ManifestError{field: deepestPointer(verr), message: verr.Message}
		}
		return Marketplace{}, &ManifestError{message: err.Error()}
	}
	var m Marketplace
	if err := json.Unmarshal(raw, &m); err != nil {
		return Marketplace{}, errpkg.Wrap("E_PARSE_MANIFEST", err, "decode marketplace")
	}
	return m, nil
}

func LoadMarketplace(ctx context.Context, repoDir string) (Marketplace, error) {
	raw, err := os.ReadFile(filepath.Join(repoDir, manifestFilename))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Marketplace{}, ErrNotAMarketplace
		}
		return Marketplace{}, errpkg.Wrap("E_MANIFEST_READ", err, "read "+manifestFilename)
	}
	return ParseMarketplace(ctx, raw)
}

func deepestPointer(verr *jsonschema.ValidationError) string {
	for len(verr.Causes) > 0 {
		verr = verr.Causes[0]
	}
	return verr.InstanceLocation
}
