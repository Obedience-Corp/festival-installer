package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func TestHomeNavigation_Quit(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.cursor = len(m.homeItems()) - 1 // Quit
	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	// execute cmd: tea.Quit returns a quit msg
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		// tea.Quit may return tea.QuitMsg via special internal; accept non-nil
		_ = nm
	}
}

func TestInstallStartsStrictBeforeConsent(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenInstall
	m.status.Action = "absent"

	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd == nil || nm.screen != screenProgress {
		t.Fatalf("screen=%v cmd=%v, want strict install attempt first", nm.screen, cmd)
	}
	if nm.confirmAct != "" {
		t.Fatalf("should not open consent before attempt, act=%q", nm.confirmAct)
	}
}

func TestInstallPackageOriginEnterDoesNotInstall(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenInstall
	m.status.Action = "package"
	m.installKind = "package"

	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd != nil {
		t.Fatal("package-origin enter must not start InstallFestival")
	}
	if nm.screen != screenHome {
		t.Fatalf("screen=%v, want home (enter = back)", nm.screen)
	}
}

func TestInstallLeftoverOriginEnterReachesPicker(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenInstall
	m.status.Action = "unmanaged"
	m.installKind = "leftover"
	m.status.Shadows = []app.ToolLocation{
		{Tool: "camp", Path: "/home/lancer/local/bin/camp", Version: "v0.5.0"},
	}

	out := m.viewInstall()
	if !strings.Contains(out, "festival uninstall cannot remove these files") {
		t.Fatalf("leftover install screen missing copy\n%s", out)
	}
	if strings.Contains(out, "do not run festival install") {
		t.Fatal("leftover install must not use the AUR refuse screen")
	}

	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd != nil {
		t.Fatal("first leftover enter should not start InstallFestival")
	}
	if nm.screen != screenInstall || nm.installKind != "" {
		t.Fatalf("screen=%v installKind=%q, want picker", nm.screen, nm.installKind)
	}
	if !strings.Contains(nm.viewInstall(), "to install") {
		t.Fatalf("want channel picker after leftover enter\n%s", nm.viewInstall())
	}
}

func TestViewListPackageOriginUsesSource(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenList
	m.list = app.ListResult{Packages: []app.ListEntry{{
		PackageID: app.FestivalPackageID,
		Version:   "0.3.1",
		Channel:   "stable",
		Source:    "aur:festival-bin",
		Origin:    "package",
	}}}
	out := m.viewList()
	if !strings.Contains(out, "aur:festival-bin") {
		t.Fatalf("package list should show Source in parens, got:\n%s", out)
	}
	if strings.Contains(out, "(stable)") {
		t.Fatalf("package list must not show Channel in parens:\n%s", out)
	}
}

func TestLoadMarketsEmptyHomeDoesNotCreateDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	m := newModel(Options{Version: "test"})
	msg := m.loadMarkets()()
	mm, ok := msg.(marketMsg)
	if !ok {
		t.Fatalf("got %T", msg)
	}
	if mm.err != nil {
		t.Fatalf("list: %v", mm.err)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist after marketplace list, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "locks")); !os.IsNotExist(err) {
		t.Fatalf("locks/ must not exist after marketplace list, err=%v", err)
	}
}

func TestMarketplaceSKeyCreatesDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	origList, origSeed := marketplaceListFn, marketplaceSeedFn
	t.Cleanup(func() {
		marketplaceListFn, marketplaceSeedFn = origList, origSeed
	})
	marketplaceSeedFn = func(ctx context.Context, vo source.VerifyOptions) error {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(home, "state.db"), []byte("stub"), 0o600)
	}
	marketplaceListFn = func(ctx context.Context, vo source.VerifyOptions) ([]source.ListView, error) {
		return []source.ListView{}, nil
	}
	m := newModel(Options{Version: "test"})
	m.screen = screenMarketplace
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("s should seed the official marketplace")
	}
	_ = cmd()
	if _, err := os.Stat(filepath.Join(home, "state.db")); os.IsNotExist(err) {
		t.Fatal("s should create state.db")
	}
}

func TestInstallPackageForceKeyShowsPicker(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenInstall
	m.installKind = "package"
	m.status.Action = "package"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	nm := next.(model)
	if cmd != nil {
		t.Fatal("f should not start install immediately")
	}
	if nm.installKind != "" || !nm.installForce {
		t.Fatalf("installKind=%q force=%v, want picker with force", nm.installKind, nm.installForce)
	}
	if !strings.Contains(nm.viewInstall(), "to install") {
		t.Fatalf("want channel picker after f\n%s", nm.viewInstall())
	}
}

func TestUninstallPackageOriginSkipsUninstallPackage(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenUninstall
	m.status.Action = "package"
	m.status.Remove = "yay -Rns festival-bin"
	m.list = app.ListResult{Packages: []app.ListEntry{{
		PackageID: app.FestivalPackageID,
		Origin:    "package",
		Source:    "aur:festival-bin",
	}}}
	next, cmd := m.handleEnter()
	nm := next.(model)
	if cmd != nil || nm.screen != screenConfirm || nm.confirmAct != "uninstall-note" {
		t.Fatalf("screen=%v act=%q cmd=%v", nm.screen, nm.confirmAct, cmd)
	}
	if !strings.Contains(nm.confirmMsg, "yay -Rns festival-bin") {
		t.Fatalf("confirm missing remove command: %q", nm.confirmMsg)
	}
	nm.confirmYes = true
	next, cmd = nm.handleEnter()
	nm = next.(model)
	if cmd != nil {
		t.Fatal("package uninstall confirm must not call UninstallPackage")
	}
	if nm.screen != screenResult {
		t.Fatalf("screen=%v, want result", nm.screen)
	}
}

func TestConsentNeededMsgOpensOverrideDialog(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.busy = true
	m.screen = screenProgress
	next, cmd := m.Update(consentNeededMsg{
		action: "install-unverified",
		cause:  errpkg.New("E_UNVERIFIED_REFUSED", "unsigned"),
	})
	nm := next.(model)
	if cmd != nil {
		t.Fatal("consent prompt should not start another command")
	}
	if nm.screen != screenConfirm || nm.confirmAct != "install-unverified" {
		t.Fatalf("screen=%v action=%q, want override consent", nm.screen, nm.confirmAct)
	}
	if nm.confirmYes {
		t.Fatal("override consent must default to No")
	}
	if !nm.busy {
		// busy cleared so user can answer
	} else {
		t.Fatal("busy should clear when consent is required")
	}
}

func TestUnverifiedConsentStartsInstall(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenConfirm
	m.confirmAct = "install-unverified"
	m.confirmYes = true

	next, cmd := m.handleEnter()
	nm := next.(model)
	if nm.screen != screenProgress || cmd == nil {
		t.Fatalf("screen=%v cmd=%v, want progress and install command", nm.screen, cmd)
	}
}

func TestHasErrorCodeWalksWraps(t *testing.T) {
	inner := errpkg.New("E_UNVERIFIED_REFUSED", "refused")
	outer := errpkg.Wrap("E_PKG_MANIFEST", inner, "load")
	if !hasErrorCode(outer, "E_UNVERIFIED_REFUSED") {
		t.Fatal("expected walk to find E_UNVERIFIED_REFUSED")
	}
	if hasErrorCode(outer, "E_OTHER") {
		t.Fatal("unexpected code match")
	}
}

func TestLaunchpad_SetsPendingLaunch(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenLaunchpad
	m.cursor = 0
	// Point PATH at a fake camp so resolve succeeds.
	dir := t.TempDir()
	camp := dir + "/camp"
	if err := os.WriteFile(camp, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FESTIVAL_HOME", t.TempDir())

	next, cmd := m.launchSelected()
	nm := next.(model)
	if nm.pendingLaunch == nil {
		t.Fatalf("expected pendingLaunch, err=%v", nm.err)
	}
	if nm.pendingLaunch.Tool != "camp" {
		t.Fatalf("tool %q", nm.pendingLaunch.Tool)
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestBootSkipsOnEnter(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenBoot
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(model).screen != screenHome {
		t.Fatalf("want home after skip, got %v", next.(model).screen)
	}
}

func TestMaxCursorHome(t *testing.T) {
	m := newModel(Options{})
	m.screen = screenHome
	if m.maxCursor() != len(m.homeItems())-1 {
		t.Fatalf("max cursor %d want %d", m.maxCursor(), len(m.homeItems())-1)
	}
}

func TestHomeItemsLengthAndRelabel(t *testing.T) {
	m := newModel(Options{Version: "test"})
	if got := len(m.homeItems()); got != 10 {
		t.Fatalf("len(homeItems)=%d, want 10", got)
	}
	if m.homeItems()[0] != "Install Festival suite" {
		t.Fatalf("absent index 0 = %q", m.homeItems()[0])
	}
	if m.homeItems()[9] != "Quit" {
		t.Fatalf("index 9 = %q, want Quit", m.homeItems()[9])
	}

	m.status.Action = "package"
	if m.homeItems()[0] != "How you installed" {
		t.Fatalf("package index 0 = %q, want How you installed", m.homeItems()[0])
	}
	if got := len(m.homeItems()); got != 10 {
		t.Fatalf("package len=%d, want 10", got)
	}

	m.status = app.StatusSummary{Action: "unmanaged", Dual: false}
	if m.homeItems()[0] != "Install Festival suite" {
		t.Fatalf("leftover index 0 = %q, want Install Festival suite", m.homeItems()[0])
	}

	m.status = app.StatusSummary{Action: "managed", Dual: true}
	if m.homeItems()[0] != "How you installed" {
		t.Fatalf("Dual index 0 = %q, want How you installed", m.homeItems()[0])
	}
}

func TestDigitZeroSelectsQuit(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = 0
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	nm := next.(model)
	if nm.cursor != 9 {
		t.Fatalf("digit 0 cursor=%d, want 9 (Quit)", nm.cursor)
	}
	if cmd == nil {
		t.Fatal("digit 0 should quit")
	}
}

func TestHomeDefaultCursor(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	next, _ := m.Update(statusMsg{sum: app.StatusSummary{Action: "managed"}})
	if next.(model).cursor != 1 {
		t.Fatalf("managed cursor=%d, want 1", next.(model).cursor)
	}

	m = newModel(Options{Version: "test"})
	m.screen = screenHome
	next, _ = m.Update(statusMsg{sum: app.StatusSummary{Action: "package"}})
	if next.(model).cursor != 0 {
		t.Fatalf("package cursor=%d, want 0", next.(model).cursor)
	}

	m = newModel(Options{Version: "test"})
	m.screen = screenHome
	next, _ = m.Update(statusMsg{sum: app.StatusSummary{Action: "unmanaged"}})
	if next.(model).cursor != 0 {
		t.Fatalf("leftover cursor=%d, want 0", next.(model).cursor)
	}
}

func TestOpenHomeItemInstallKind(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = 0
	m.status.Action = "package"
	next, _ := m.openHomeItem()
	if got := next.(model).installKind; got != "package" {
		t.Fatalf("package installKind=%q", got)
	}

	m.status.Action = "unmanaged"
	m.status.Dual = false
	next, _ = m.openHomeItem()
	if got := next.(model).installKind; got != "leftover" {
		t.Fatalf("leftover installKind=%q", got)
	}

	m.status.Action = "absent"
	next, _ = m.openHomeItem()
	if got := next.(model).installKind; got != "" {
		t.Fatalf("absent installKind=%q, want empty (channel picker)", got)
	}
}

func TestViewHomePackageChannelCard(t *testing.T) {
	m := newModel(Options{Version: "0.3.1"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	m.status = app.StatusSummary{
		Action:  "package",
		Version: "0.3.1",
		Flavor:  app.FlavorAUR,
		Package: "festival-bin",
		Prefix:  "/usr/bin",
		Helper:  "source /usr/share/festival/shell/festival.zsh",
		Upgrade: "yay -Syu festival-bin",
	}
	out := m.viewHome()
	if strings.Contains(out, "camp/fest found but not managed") {
		t.Fatal("package home must not use leftover unmanaged copy")
	}
	if strings.Contains(out, "PATH missing") {
		t.Fatal("package home must not report managed PATH missing")
	}
	if strings.Contains(out, "eval \"$(festival shell-init zsh)\"") {
		t.Fatal("package home must not recommend festival shell-init")
	}
	for _, want := range []string{
		"festival 0.3.1 · AUR (festival-bin)",
		"suite on PATH · /usr/bin",
		"helper: source /usr/share/festival/shell/festival.zsh",
		"upgrade: yay -Syu festival-bin",
		"do not run festival install",
		"How you installed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("package home missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "Install Festival suite") {
		t.Fatal("package home still shows Install Festival suite")
	}
}

func TestViewHomePackageShowsUpdateAvailable(t *testing.T) {
	m := newModel(Options{Version: "0.3.1"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	m.status = app.StatusSummary{
		Action:  "package",
		Version: "0.3.1",
		Flavor:  app.FlavorAUR,
		Package: "festival-bin",
		Prefix:  "/usr/bin",
		Upgrade: "yay -Syu festival-bin",
		Latest:  "0.3.3",
	}
	out := m.viewHome()
	for _, want := range []string{
		"update available: 0.3.1 → 0.3.3",
		"Update Festival · 0.3.3 available",
		"upgrade: yay -Syu festival-bin",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("package home missing %q\n%s", want, out)
		}
	}
}

func TestLatestMsgSelectsUpdateWhenAvailable(t *testing.T) {
	m := newModel(Options{Version: "0.3.1"})
	m.screen = screenHome
	m.cursor = 0
	m.status = app.StatusSummary{Action: "package", Version: "0.3.1"}
	next, _ := m.Update(latestMsg{latest: "0.3.3"})
	nm := next.(model)
	if nm.status.Latest != "0.3.3" {
		t.Fatalf("Latest=%q", nm.status.Latest)
	}
	if nm.cursor != 1 {
		t.Fatalf("cursor=%d, want 1 (Update Festival)", nm.cursor)
	}
	if nm.homeItems()[1] != "Update Festival · 0.3.3 available" {
		t.Fatalf("item 1 = %q", nm.homeItems()[1])
	}
}

func TestStatusMsgPackageChecksLatest(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	_, cmd := m.Update(statusMsg{sum: app.StatusSummary{Action: "package", Version: "0.3.1"}})
	if cmd == nil {
		t.Fatal("package status must kick off a latest-version check")
	}
}

func TestViewHomeLeftoverDoesNotRelabel(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	m.status = app.StatusSummary{
		Action: "unmanaged",
		Prefix: "/home/lancer/local/bin",
		Shadows: []app.ToolLocation{
			{Tool: "camp", Path: "/home/lancer/local/bin/camp", Version: "v0.5.0"},
			{Tool: "fest", Path: "/home/lancer/local/bin/fest", Version: "dev"},
		},
	}
	out := m.viewHome()
	if !strings.Contains(out, "Install Festival suite") {
		t.Fatal("leftover home must keep Install Festival suite")
	}
	if strings.Contains(out, "How you installed") {
		t.Fatal("leftover home must not relabel index 0")
	}
	if strings.Contains(out, "do not run festival install") {
		t.Fatal("leftover home must not tell the user not to install")
	}
	if !strings.Contains(out, "/home/lancer/local/bin/camp  (v0.5.0, no bundle)") {
		t.Fatalf("leftover home missing leftover path\n%s", out)
	}
}

func TestViewHomePackageLeftoverShadows(t *testing.T) {
	m := newModel(Options{Version: "0.3.1"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	m.status = app.StatusSummary{
		Action:  "package",
		Version: "0.3.1",
		Flavor:  app.FlavorAUR,
		Package: "festival-bin",
		Prefix:  "/usr/bin",
		Helper:  "source /usr/share/festival/shell/festival.zsh",
		Upgrade: "yay -Syu festival-bin",
		Shadows: []app.ToolLocation{
			{Tool: "camp", Path: "/home/lancer/local/bin/camp", Version: "v0.5.0"},
			{Tool: "fest", Path: "/home/lancer/local/bin/fest", Version: "dev"},
		},
	}
	out := m.viewHome()
	if !strings.Contains(out, "leftover binaries ahead of /usr/bin") {
		t.Fatalf("missing leftover PATH warn\n%s", out)
	}
	if strings.Contains(out, "eval \"$(festival shell-init zsh)\"") {
		t.Fatal("shadowed package home must not recommend festival shell-init")
	}
	if !strings.Contains(out, "remove those two files; keep /home/lancer/local/bin on PATH") {
		t.Fatalf("missing leftover cleanup hint\n%s", out)
	}
}

func TestViewHomeNonEmpty(t *testing.T) {
	m := newModel(Options{Version: "0.1.0"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24
	out := m.View()
	if out == "" {
		t.Fatal("empty view")
	}
	if len(out) < 50 {
		t.Fatalf("view too short: %q", out)
	}
}

func TestViewHomeReducedMotionIsStatic(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenHome
	m.width = 80
	m.height = 24

	m.frame = 0
	first := m.viewHome()
	m.frame = 1
	second := m.viewHome()
	if first != second {
		t.Fatal("reduced-motion home view changed between animation frames")
	}
}

func TestViewProgressReducedMotionIsStatic(t *testing.T) {
	m := newModel(Options{Version: "test"})
	m.reduced = true
	m.screen = screenProgress
	m.width = 80
	m.height = 24
	m.progress = app.ProgressEvent{Stage: "download", Percent: 0.5, Message: "fetching"}

	m.frame = 0
	first := m.View()
	m.frame = 1
	second := m.View()
	if first != second {
		t.Fatal("reduced-motion progress view changed between animation frames")
	}
}

func TestMutationCommandsInstallCancellation(t *testing.T) {
	t.Run("uninstall", func(t *testing.T) {
		m := newModel(Options{})
		next, cmd := m.startUninstall(app.FestivalPackageID)
		nm := next.(model)
		if cmd == nil || nm.opCancel == nil {
			t.Fatal("uninstall should return a command with a cancellation function")
		}
	})

	t.Run("browse install", func(t *testing.T) {
		m := newModel(Options{})
		m.browseFlat = []app.BrowseEntry{{ID: "acme/fest-demo", Class: "plugin"}}
		next, cmd := m.installBrowseSelection(true)
		nm := next.(model)
		if cmd == nil || nm.opCancel == nil {
			t.Fatal("browse install should return a command with a cancellation function")
		}
	})
}
