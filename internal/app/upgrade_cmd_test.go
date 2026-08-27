package app

import (
	"reflect"
	"testing"
)

func TestPackageUpgradeAvailable(t *testing.T) {
	if !PackageUpgradeAvailable(UpdateResult{Action: "package", Version: "0.3.3", Latest: "0.3.4"}) {
		t.Fatal("0.3.3 < 0.3.4 must be available")
	}
	if PackageUpgradeAvailable(UpdateResult{Action: "package", Version: "0.3.4", Latest: "0.3.4"}) {
		t.Fatal("current must not be available")
	}
	if PackageUpgradeAvailable(UpdateResult{Action: "upgraded", Version: "0.3.3", Latest: "0.3.4"}) {
		t.Fatal("managed upgraded is not a package-manager upgrade")
	}
}

func TestParseUpgradeArgv(t *testing.T) {
	tests := []struct {
		cmd      string
		wantTool string
		wantArgs []string
		ok       bool
	}{
		{cmd: "yay -Syu festival-bin", wantTool: "yay", wantArgs: []string{"-Syu", "festival-bin"}, ok: true},
		{cmd: "brew upgrade --cask festival", wantTool: "brew", wantArgs: []string{"upgrade", "--cask", "festival"}, ok: true},
		{cmd: "npm install -g @obedience-corp/festival@latest", wantTool: "npm", wantArgs: []string{"install", "-g", "@obedience-corp/festival@latest"}, ok: true},
		{cmd: "https://github.com/Obedience-Corp/festival/releases/latest", ok: false},
		{cmd: "reinstall obedience-festival from https://github.com/Obedience-Corp/festival/releases/latest", ok: false},
		{cmd: "curl -fsSL https://example/install.sh | bash", ok: false},
		{cmd: "", ok: false},
	}
	for _, tc := range tests {
		tool, args, ok := ParseUpgradeArgv(tc.cmd)
		if ok != tc.ok {
			t.Errorf("%q ok=%v, want %v", tc.cmd, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if tool != tc.wantTool || !reflect.DeepEqual(args, tc.wantArgs) {
			t.Errorf("%q -> %q %q, want %q %q", tc.cmd, tool, args, tc.wantTool, tc.wantArgs)
		}
	}
}
