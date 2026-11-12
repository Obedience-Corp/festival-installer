package app

import (
	"encoding/json"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
	"github.com/Obedience-Corp/festival-installer/internal/verify/minisigntest"
)

func TestAppReceiptCodecRoundTrip(t *testing.T) {
	cases := []appReceipt{
		{Format: "app-bundle", Location: "/Applications/Festival App.app", Executable: "/Applications/Festival App.app/Contents/MacOS/festival-app", Mode: 0o755},
		{Format: "nsis", Location: `C:\Users\u\AppData\Local\Festival App`, Executable: `C:\Users\u\AppData\Local\Festival App\festival-app.exe`, Mode: 0o666},
		{Format: "deb", Location: "dpkg:festival-app", Executable: "/usr/bin/festival-app", Mode: 0o755, Adopted: true},
		{Format: "appimage", Location: "/home/u/.local/bin/festival-app", Executable: "/home/u/.local/bin/festival-app", Mode: 0o755},
	}
	for _, in := range cases {
		in.Version, in.Source, in.Channel, in.BundleID = "0.1.8", "official-obey", "stable", AppBundleID
		in.ArtifactSHA256, in.ManifestURL = "ab"+in.Format, "https://example.invalid/x"
		rec := appReceiptToRecord(in)
		if rec.PackageID != AppPackageID || len(rec.OwnedFiles) != 1 || rec.OwnedFiles[0].Hash != "" || rec.OwnedFiles[0].Path != in.Executable {
			t.Fatalf("%s: record shape %+v", in.Format, rec)
		}
		want := map[string]bool{"app.format": true, "app.location": true, "app.bundle_id": true, "app.artifact_sha256": true, "app.adopted": true, "manifest_url": true}
		if len(rec.Metadata) != len(want) {
			t.Fatalf("%s: metadata keys = %v", in.Format, rec.Metadata)
		}
		for k := range rec.Metadata {
			if !want[k] {
				t.Fatalf("%s: unexpected metadata key %q", in.Format, k)
			}
		}
		if _, has := rec.Metadata["install_journal_id"]; has {
			t.Fatal("install_journal_id must never be written")
		}
		out, err := appReceiptFromRecord(rec)
		if err != nil || out != in {
			t.Fatalf("%s: round trip %+v %v, want %+v", in.Format, out, err, in)
		}
	}
}

func TestAppReceiptCodecRejectsForeignShape(t *testing.T) {
	good := appReceiptToRecord(appReceipt{Format: "appimage", Location: "/h/festival-app", Executable: "/h/festival-app", Mode: 0o755})
	noFormat := good
	noFormat.Metadata = map[string]string{"app.location": "/h/festival-app"}
	twoFiles := good
	twoFiles.OwnedFiles = append(append([]receipts.OwnedFile{}, good.OwnedFiles...), receipts.OwnedFile{Path: "/h/other"})
	for name, rec := range map[string]receipts.Receipt{"no app.format": noFormat, "two file rows": twoFiles} {
		if _, err := appReceiptFromRecord(rec); errpkg.Code(err) != "E_APP_FOREIGN" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAppStatusAbsentMarshalsNulls(t *testing.T) {
	raw, err := json.Marshal(StatusApp{})
	want := `{"installed":false,"version":null,"receipt_version":null,"format":null,"location":null,"adopted":null,"updated_by_app":null,"latest":null,"update_available":null,"note":null}`
	if err != nil || string(raw) != want {
		t.Fatalf("got %s %v", raw, err)
	}
}

func TestAppStatusToolsListUnchanged(t *testing.T) {
	want := []string{"camp", "fest", "festival", "obey", "ob"}
	if len(statusTools) != len(want) {
		t.Fatalf("statusTools = %v", statusTools)
	}
	for i := range want {
		if statusTools[i] != want[i] {
			t.Fatalf("statusTools = %v", statusTools)
		}
	}
}

func TestAppUpdaterKeySeamRestores(t *testing.T) {
	s, err := minisigntest.NewSigner()
	if err != nil {
		t.Fatal(err)
	}
	k, err := verify.ParseTauriPublicKey(s.TauriPublicKey())
	if err != nil {
		t.Fatal(err)
	}
	restore := WithUpdaterKeyForTest(k)
	if appUpdaterKey().KeyIDString() != k.KeyIDString() {
		t.Fatal("seam did not swap the key")
	}
	restore()
	if appUpdaterKey().KeyIDString() != verify.FestivalAppUpdaterKeyID {
		t.Fatal("seam did not restore the pinned key")
	}
}
