package tui

import (
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
)

// homeItemID names a home menu entry independently of its position. Every
// lookup of "what does this row do" goes through an id, so inserting or
// reordering entries cannot silently rewire the menu, which a switch over
// cursor indices did without any test noticing.
type homeItemID string

const (
	homeObey        homeItemID = "obey"
	homeTour        homeItemID = "tour"
	homeInstall     homeItemID = "install"
	homeUpdate      homeItemID = "update"
	homeList        homeItemID = "list"
	homeBrowse      homeItemID = "browse"
	homeUninstall   homeItemID = "uninstall"
	homeMarketplace homeItemID = "marketplace"
	homeDoctor      homeItemID = "doctor"
	homeShell       homeItemID = "shell"
	homeLaunchpad   homeItemID = "launchpad"
	homeDiscord     homeItemID = "discord"
	homeQuit        homeItemID = "quit"
)

// homeItem is one home menu row. booth is the index into
// anim.DefaultHomeBooths that lights up while the row is selected, so the
// ambient strip tracks the menu without a second position table to keep in
// sync.
type homeItem struct {
	id    homeItemID
	label string
	booth int
}

func (m model) homeMenu() []homeItem {
	items := []homeItem{
		{id: homeLaunchpad, label: "Open Camp & Fest", booth: 0},
		{id: homeObey, label: "Obey agent sessions", booth: 0},
		{id: homeBrowse, label: "Explore packages", booth: 1},
		{id: homeUpdate, label: "Update Festival", booth: 0},
		{id: homeList, label: "Installed packages", booth: 1},
		{id: homeInstall, label: "Install Festival suite", booth: 0},
		{id: homeDoctor, label: "Check setup", booth: 3},
		{id: homeShell, label: "Terminal setup", booth: 4},
		{id: homeTour, label: "Getting started", booth: 0},
		{id: homeMarketplace, label: "Marketplaces", booth: 2},
		{id: homeUninstall, label: "Uninstall package", booth: 1},
		{id: homeDiscord, label: "Community", booth: 0},
		{id: homeQuit, label: "Quit", booth: 0},
	}
	if m.status.Action == "managed" {
		items[m.indexOfIn(items, homeInstall)].label = "Festival suite details"
	}
	if m.statusErr != nil || m.status.Setup.Unknown() || m.status.Action == "" {
		items[m.indexOfIn(items, homeInstall)].label = "Check suite installation"
	}
	if m.status.Action == "package" || m.status.Dual {
		items[m.indexOfIn(items, homeInstall)].label = "How you installed"
	}
	if m.updateAvailable() {
		items[m.indexOfIn(items, homeUpdate)].label = "Update Festival · " + m.status.Latest + " available"
	}
	return items
}

func (m model) homeItems() []string {
	menu := m.homeMenu()
	labels := make([]string, len(menu))
	for i, it := range menu {
		labels[i] = it.label
	}
	return labels
}

// homeIndexOf is the cursor position of id, or -1 when the menu does not
// currently carry that entry.
func (m model) homeIndexOf(id homeItemID) int {
	return m.indexOfIn(m.homeMenu(), id)
}

func (m model) indexOfIn(items []homeItem, id homeItemID) int {
	for i, it := range items {
		if it.id == id {
			return i
		}
	}
	return -1
}

// homeItemAt is the entry under a cursor position, guarding the bounds so a
// stale cursor cannot index out of the menu.
func (m model) homeItemAt(cursor int) (homeItem, bool) {
	menu := m.homeMenu()
	if cursor < 0 || cursor >= len(menu) {
		return homeItem{}, false
	}
	return menu[cursor], true
}

func (m model) updateAvailable() bool {
	ver := strings.TrimPrefix(m.status.Version, "v")
	latest := strings.TrimPrefix(m.status.Latest, "v")
	return latest != "" && ver != "" && installer.VersionLess(ver, latest)
}

func (m model) defaultHomeCursor() int {
	if m.updateAvailable() {
		if i := m.homeIndexOf(homeUpdate); i >= 0 {
			return i
		}
	}
	if m.status.Action == "absent" && !m.status.Setup.Unknown() && m.statusErr == nil {
		return m.homeIndexOf(homeInstall)
	}
	return 0
}

// moveToUpdateEntry parks the cursor on Update once a newer release is known,
// but only when the user has not moved it off the entry the home screen opened
// on. Nudging a cursor the user placed themselves would be rude.
func (m model) moveToUpdateEntry(cursor int) int {
	update := m.homeIndexOf(homeUpdate)
	if update < 0 {
		return cursor
	}
	if cursor == 0 || cursor == m.homeIndexOf(homeInstall) {
		return update
	}
	return cursor
}
