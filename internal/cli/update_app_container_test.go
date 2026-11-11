//go:build container_fs && linux

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/apptest/appfixture"
	"github.com/Obedience-Corp/festival-installer/internal/cli"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func runOutdated(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	dropHostFestivalPATH(t)
	root := &cobra.Command{Use: "festival", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cli.NewOutdatedCommand())
	cli.WrapJSONErrors(root)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"outdated"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

type cliAppEnv struct {
	t          *testing.T
	cat        *appfixture.Catalog
	rel        *appfixture.Releases
	releases   []metadata.Release
	registered bool
}

func newCLIAppEnv(t *testing.T) *cliAppEnv {
	t.Helper()
	e := &cliAppEnv{t: t, cat: appfixture.NewCatalog(t), rel: appfixture.NewReleases(t)}
	t.Cleanup(app.WithUpdaterKeyForTest(e.rel.Key))
	t.Cleanup(source.WithPinnedKeyStoreForTest(e.cat.Verify.KeyStore))
	return e
}

func (e *cliAppEnv) publish(version string, body []byte) {
	e.t.Helper()
	e.releases = append(e.releases, appfixture.Release(version, "stable", e.rel.Artifact("appimage", version, body)))
	e.cat.Publish(appfixture.Manifest(e.releases...), appfixture.PublishOptions{})
	if !e.registered {
		e.cat.Register("official-obey")
		e.registered = true
	}
}

func (e *cliAppEnv) install() app.AppResult {
	e.t.Helper()
	res, err := app.InstallApp(context.Background(), app.AppInstallOptions{Format: "appimage", Verify: source.DefaultVerifyOptions(io.Discard, false)})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

type updateAppData struct {
	Package         string `json:"package"`
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
	Format          string `json:"format"`
	Location        string `json:"location"`
	Adopted         bool   `json:"adopted"`
}

func TestAppOutdatedJSONListsRow(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	e.install()
	e.publish("0.2.1", []byte("appimage 0.2.1"))
	out, errOut, err := runOutdated(t, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	var data struct {
		Packages []struct {
			Package, Installed, Latest, Status string
		} `json:"packages"`
	}
	dataOf(t, out, &data)
	for _, p := range data.Packages {
		if p.Package == "obedience-corp/festival-app" {
			if p.Installed != "0.2.0" || p.Latest != "0.2.1" || p.Status != "available" {
				t.Fatalf("row = %+v", p)
			}
			t.Logf("outdated --json App row: %+v", p)
			return
		}
	}
	t.Fatalf("no App row in %s", out)
}

func TestAppUpdateJSONCurrent(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	e.install()
	out, errOut, err := runInstaller(t, "update", "app", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	var d updateAppData
	dataOf(t, out, &d)
	if d.Package != "obedience-corp/festival-app" || d.Version != "0.2.0" || d.PreviousVersion != "0.2.0" || d.Format != "appimage" || d.Location == "" || d.Adopted {
		t.Fatalf("%+v", d)
	}
	if e.rel.Downloads("appimage", "0.2.0") != 1 {
		t.Fatalf("the artifact was downloaded %d times; the update must not fetch it again", e.rel.Downloads("appimage", "0.2.0"))
	}
	t.Logf("update app --json (current): %s", strings.TrimSpace(out))
}

func TestAppUpdateNotInstalledWarns(t *testing.T) {
	newCLIAppEnv(t)
	out, errOut, err := runInstaller(t, "update", "app", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	var env struct {
		OK       bool     `json:"ok"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK || len(env.Warnings) != 1 ||
		env.Warnings[0] != "Festival App is not installed through festival; run festival install app" {
		t.Fatalf("%s %v", out, err)
	}
}

func TestAppUpdateRejectsNonStableChannel(t *testing.T) {
	newCLIAppEnv(t)
	_, _, err := runInstaller(t, "update", "app", "--channel", "dev", "--json")
	if err == nil || !strings.Contains(err.Error(), "E_APP_NOT_AVAILABLE") || !strings.Contains(err.Error(), "published on the stable channel only") {
		t.Fatalf("err = %v", err)
	}
}

func TestAppUpdateUpgradesAppImage(t *testing.T) {
	e := newCLIAppEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	first := e.install()
	if first.Version != "0.2.0" || first.Format != "appimage" {
		t.Fatalf("%+v", first)
	}
	e.publish("0.2.1", []byte("appimage 0.2.1 with different bytes"))
	out, errOut, err := runInstaller(t, "update", "app", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, errOut)
	}
	var d updateAppData
	dataOf(t, out, &d)
	if d.Version != "0.2.1" || d.PreviousVersion != "0.2.0" || d.Format != "appimage" {
		t.Fatalf("%+v", d)
	}
	got, err := os.ReadFile(d.Location)
	if err != nil || string(got) != "appimage 0.2.1 with different bytes" {
		t.Fatalf("binary = %q %v", got, err)
	}
	var st struct {
		App struct {
			ReceiptVersion string `json:"receipt_version"`
		} `json:"app"`
	}
	sout, serr, err := runInstaller(t, "status", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, serr)
	}
	dataOf(t, sout, &st)
	if st.App.ReceiptVersion != "0.2.1" {
		t.Fatalf("status.app.receipt_version = %q", st.App.ReceiptVersion)
	}
	t.Logf("update app --json (upgrade): %s", strings.TrimSpace(out))
}
