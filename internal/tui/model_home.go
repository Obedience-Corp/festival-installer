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
		{id: homeTour, label: "Getting started", booth: 0},
		{id: homeInstall, label: "Install Festival suite", booth: 0},
		{id: homeUpdate, label: "Update Festival", booth: 0},
		{id: homeList, label: "Installed packages", booth: 1},
		{id: homeBrowse, label: "Browse catalog", booth: 1},
		{id: homeUninstall, label: "Uninstall package", booth: 1},
		{id: homeMarketplace, label: "Marketplaces", booth: 2},
		{id: homeDoctor, label: "Doctor", booth: 3},
		{id: homeShell, label: "Shell / PATH setup", booth: 4},
		{id: homeLaunchpad, label: "Launchpad (camp / fest tools)", booth: 0},
		{id: homeQuit, label: "Quit", booth: 0},
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
	if m.updateAvailable() || (m.status.Action == "managed" && !m.status.Dual) {
		if i := m.homeIndexOf(homeUpdate); i >= 0 {
			return i
		}
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
