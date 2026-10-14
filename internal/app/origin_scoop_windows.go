package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// scoopOwns accepts a normal Scoop shim only when its metadata points to the
// matching executable in this installation's festival app and that installed
// manifest declares the executable. A path ending in shims alone is not proof.
func scoopOwns(path string) bool {
	shimDir := filepath.Dir(path)
	if !strings.EqualFold(filepath.Base(shimDir), "shims") {
		return false
	}
	name := filepath.Base(path)
	if !strings.EqualFold(filepath.Ext(name), ".exe") {
		return false
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if !strings.EqualFold(base, "camp") && !strings.EqualFold(base, "fest") && !strings.EqualFold(base, "festival") {
		return false
	}
	shim, err := os.ReadFile(filepath.Join(shimDir, strings.ToLower(base)+".shim"))
	if err != nil {
		return false
	}
	target := scoopShimTarget(string(shim))
	appDir := filepath.Join(filepath.Dir(shimDir), "apps", "festival", "current")
	wantTarget := filepath.Join(appDir, name)
	if !filepath.IsAbs(target) || !strings.EqualFold(filepath.Clean(target), filepath.Clean(wantTarget)) || !fileExists(target) {
		return false
	}
	manifest, err := os.ReadFile(filepath.Join(appDir, "scoop-manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		manifest, err = os.ReadFile(filepath.Join(appDir, "manifest.json"))
	}
	return err == nil && scoopManifestHasBinary(manifest, name)
}

func scoopShimTarget(data string) string {
	for _, line := range strings.Split(strings.TrimPrefix(data, "\ufeff"), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "path") {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		return value
	}
	return ""
}

func scoopManifestHasBinary(data []byte, name string) bool {
	var manifest struct {
		Bin          []string `json:"bin"`
		Architecture struct {
			AMD64 struct {
				Bin []string `json:"bin"`
			} `json:"64bit"`
		} `json:"architecture"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}
	for _, bin := range append(manifest.Bin, manifest.Architecture.AMD64.Bin...) {
		if strings.EqualFold(filepath.Base(filepath.FromSlash(bin)), name) {
			return true
		}
	}
	return false
}
