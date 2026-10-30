package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
)

func TestUpdateAllRejectsOwnershipAndChannelOverrides(t *testing.T) {
	for _, args := range [][]string{{"--all", "obey"}, {"--all", "--channel", "rc"}, {"--all", "--force"}} {
		cmd := NewUpdateCommand()
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestUpdatePartialJSONRetainsResults(t *testing.T) {
	var out bytes.Buffer
	err := updateEnvelope(&out, "outdated", app.UpdatesResult{Packages: []app.PackageUpdate{{Package: "one", Status: app.UpdateCheckFailed, Detail: "offline"}}}, true)
	if err == nil {
		t.Fatal("failed check returned success")
	}
	var envelope jsonout.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Data == nil || envelope.Error.Code != "E_UPDATE_PARTIAL" {
		t.Fatalf("bad envelope: %+v", envelope)
	}
}
