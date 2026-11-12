//go:build container_fs && linux

package cli_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

type appStatusData struct {
	Tools []struct {
		Tool string `json:"tool"`
	} `json:"tools"`
	App struct {
		Installed bool    `json:"installed"`
		Version   *string `json:"version"`
		Format    *string `json:"format"`
	} `json:"app"`
}

func TestAppStatusJSONKeepsToolsAndAddsApp(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "installer")
	isolatedStatusEnv(t, home)
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(os.Getenv("HOME"), ".local", "bin", "festival-app") // linuxPlatform's AppImage target
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, body, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	err = receipts.Write(ctx, db.Raw(), receipts.Receipt{
		PackageID: app.AppPackageID, Version: "0.2.0", Source: "official-obey", Channel: "stable",
		OwnedFiles: []receipts.OwnedFile{{Path: exe, Hash: "", Mode: 0o755}},
		Metadata: map[string]string{
			"app.format": "appimage", "app.location": exe, "app.bundle_id": app.AppBundleID,
			"app.artifact_sha256": hex.EncodeToString(sum[:]), "app.adopted": "false",
			"manifest_url": "https://example.invalid/Festival.App_0.2.0_amd64.AppImage",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"status", "--json"}, {"status", "--json", "--plugin", "obedience-corp/camp-app"}} {
		out, errOut, err := runInstaller(t, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		var data appStatusData
		dataOf(t, out, &data)
		if !data.App.Installed || data.App.Version == nil || *data.App.Version != "0.2.0" || data.App.Format == nil || *data.App.Format != "appimage" {
			t.Fatalf("%v: app = %+v", args, data.App)
		}
		if len(args) == 2 {
			want := []string{"camp", "fest", "festival", "obey", "ob"}
			if len(data.Tools) != len(want) {
				t.Fatalf("tools = %+v", data.Tools)
			}
			for i, name := range want {
				if data.Tools[i].Tool != name {
					t.Fatalf("tools[%d] = %q, want %q", i, data.Tools[i].Tool, name)
				}
			}
		}
		var raw map[string]any
		_ = json.Unmarshal([]byte(out), &raw)
		t.Logf("%v app object present: %v", args, raw["data"].(map[string]any)["app"])
	}
}
