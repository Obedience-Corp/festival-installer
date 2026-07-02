package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

type SchemaError struct {
	field   string
	message string
}

func (e *SchemaError) Error() string {
	if e.field == "" {
		return "schema invalid: " + e.message
	}
	return "schema invalid at " + e.field + ": " + e.message
}

func (e *SchemaError) Field() string { return e.field }

// ErrSchemaInvalid is the sentinel callers compare with errors.Is.
// Concrete errors are *SchemaError values that wrap this sentinel.
var ErrSchemaInvalid = errpkg.New("E_SCHEMA_INVALID", "metadata document failed schema validation")

func (e *SchemaError) Is(target error) bool {
	return target == ErrSchemaInvalid
}

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

func ensureSchemas() error {
	compileOnce.Do(func() {
		compiled = map[string]*jsonschema.Schema{}
		compiler := jsonschema.NewCompiler()
		for _, name := range []string{"source", "index", "manifest"} {
			data, err := schemaFS.ReadFile("schemas/" + name + ".schema.json")
			if err != nil {
				compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "read "+name)
				return
			}
			if err := compiler.AddResource(name+".json", strings.NewReader(string(data))); err != nil {
				compileErr = errpkg.Wrap("E_SCHEMA_LOAD", err, "add "+name)
				return
			}
			sch, err := compiler.Compile(name + ".json")
			if err != nil {
				compileErr = errpkg.Wrap("E_SCHEMA_COMPILE", err, "compile "+name)
				return
			}
			compiled[name] = sch
		}
	})
	return compileErr
}

func validate(schemaName string, raw []byte) error {
	if err := ensureSchemas(); err != nil {
		return err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return &SchemaError{message: "invalid JSON: " + err.Error()}
	}
	if err := compiled[schemaName].Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			field := jsonPointer(verr)
			return &SchemaError{field: field, message: verr.Message}
		}
		return &SchemaError{message: err.Error()}
	}
	return nil
}

func jsonPointer(verr *jsonschema.ValidationError) string {
	// Drill into the deepest cause for the most specific field path.
	for len(verr.Causes) > 0 {
		verr = verr.Causes[0]
	}
	return verr.InstanceLocation
}

func parseSource(ctx context.Context, raw []byte) (Source, error) {
	if err := ctx.Err(); err != nil {
		return Source{}, errpkg.Wrap("E_PARSE_CTX", err, "context cancelled")
	}
	if err := validate("source", raw); err != nil {
		return Source{}, err
	}
	var s Source
	if err := json.Unmarshal(raw, &s); err != nil {
		return Source{}, errpkg.Wrap("E_PARSE_SOURCE", err, "decode source")
	}
	return s, nil
}

func parseIndex(ctx context.Context, raw []byte) (Index, error) {
	if err := ctx.Err(); err != nil {
		return Index{}, errpkg.Wrap("E_PARSE_CTX", err, "context cancelled")
	}
	if err := validate("index", raw); err != nil {
		return Index{}, err
	}
	var i Index
	if err := json.Unmarshal(raw, &i); err != nil {
		return Index{}, errpkg.Wrap("E_PARSE_INDEX", err, "decode index")
	}
	return i, nil
}

func parseManifest(ctx context.Context, raw []byte) (PackageManifest, error) {
	if err := ctx.Err(); err != nil {
		return PackageManifest{}, errpkg.Wrap("E_PARSE_CTX", err, "context cancelled")
	}
	if err := validate("manifest", raw); err != nil {
		return PackageManifest{}, err
	}
	var m PackageManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return PackageManifest{}, errpkg.Wrap("E_PARSE_MANIFEST", err, "decode manifest")
	}
	return m, nil
}
