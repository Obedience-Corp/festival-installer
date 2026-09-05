package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenHome:
		return m.openHomeItem()
	case screenInstall:
		switch m.installKind {
		case "package":
			m.screen = screenHome
			m.installKind = ""
			return m, nil
		case "leftover":
			m.installKind = ""
			return m, nil
		default:
			// Try strict first; prompt only when VER-01 refuses unsigned content.
			return m.startInstall(false)
		}
	case screenUpdate:
		return m.startUpdate(false)
	case screenList:
		return m, nil
	case screenBrowse:
		if len(m.browseFlat) == 0 {
			return m, nil
		}
		return m.installBrowseSelection(false)
	case screenUninstall:
		if len(m.list.Packages) == 0 {
			return m, nil
		}
		pkg := m.list.Packages[m.cursor]
		m.confirmYes = false
		m.confirmArg = pkg.PackageID
		m.screen = screenConfirm
		switch pkg.Origin {
		case "package":
			remove := m.status.Remove
			if remove == "" {
				remove = pkg.Source
			}
			m.confirmMsg = "Suite is owned by the package manager. Uninstall with: " + remove + "\n\nfestival uninstall does not delete package-manager files."
			if m.status.Dual {
				m.confirmAct = "uninstall"
				m.confirmMsg += "\n\nRemove the hub-managed copy (receipts) only?"
			} else {
				m.confirmAct = "uninstall-note"
			}
		case "leftover":
			m.confirmMsg = "leftover binaries at " + m.status.Prefix + "; festival uninstall cannot remove them."
			m.confirmAct = "uninstall-note"
		default:
			m.confirmMsg = "Uninstall " + pkg.PackageID + "?"
			m.confirmAct = "uninstall"
		}
		return m, nil
	case screenConfirm:
		if !m.confirmYes {
			m.screen = screenHome
			m.err = nil
			return m, nil
		}
		switch m.confirmAct {
		case "uninstall-note":
			m.screen = screenResult
			m.resultOK = true
			m.resultTitle = "Uninstall"
			m.resultBody = m.confirmMsg
			m.confirmAct = ""
			return m, nil
		case "uninstall":
			return m.startUninstall(m.confirmArg)
		case "install-unverified":
			return m.startInstall(true)
		case "update-unverified":
			return m.startUpdate(true)
		case "browse-install-unverified":
			return m.installBrowseSelection(true)
		}
		m.screen = screenHome
		return m, nil
	case screenMarketplace:
		if m.marketMode == "add" {
			return m.submitMarketplaceAdd()
		}
		ctx := m.ctx
		if m.cursor >= len(m.markets) {
			// refresh all
			return m, tea.Batch(func() tea.Msg {
				_, err := app.MarketplaceRefresh(ctx, "", tuiVerifyOptions(nil, false))
				if err != nil {
					return marketMsg{err: err}
				}
				views, err := app.MarketplaceList(ctx, tuiVerifyOptions(nil, false))
				return marketMsg{views: views, err: err}
			})
		}
		// remove selected marketplace? use 'd' - for enter, refresh single
		name := m.markets[m.cursor].Name
		return m, tea.Batch(func() tea.Msg {
			_, err := app.MarketplaceRefresh(ctx, name, tuiVerifyOptions(nil, false))
			if err != nil {
				return marketMsg{err: err}
			}
			views, err := app.MarketplaceList(ctx, tuiVerifyOptions(nil, false))
			return marketMsg{views: views, err: err}
		})
	case screenResult:
		m.screen = screenHome
		m.err = nil
		return m, nil
	case screenDoctor, screenShell:
		m.screen = screenHome
		return m, nil
	case screenLaunchpad:
		return m.launchSelected()
	}
	return m, nil
}

func (m model) openHomeItem() (tea.Model, tea.Cmd) {
	item, ok := m.homeItemAt(m.cursor)
	if !ok {
		return m, nil
	}
	switch item.id {
	case homeInstall:
		m.screen = screenInstall
		m.channelIdx = 0
		m.installKind = ""
		m.installForce = false
		switch {
		case m.status.Action == "package" || m.status.Dual:
			m.installKind = "package"
		case m.status.Action == "unmanaged":
			m.installKind = "leftover"
		}
		return m, nil
	case homeUpdate:
		m.screen = screenUpdate
		return m.startUpdate(false)
	case homeList:
		m.screen = screenList
		return m, m.loadList()
	case homeBrowse:
		m.screen = screenBrowse
		m.productF, m.kindF = "", ""
		return m, m.loadBrowse("", "")
	case homeUninstall:
		m.screen = screenUninstall
		return m, m.loadList()
	case homeMarketplace:
		m.screen = screenMarketplace
		m.marketMode = "list"
		return m, m.loadMarkets()
	case homeDoctor:
		m.screen = screenDoctor
		return m, m.loadDoctor()
	case homeShell:
		m.screen = screenShell
		g, err := app.ShellGuidanceFor(m.ctx, "zsh")
		m.shellBin = g.Bin
		m.shellOnPath = g.OnPath
		m.shellSnippet = g.Snippet
		if err != nil {
			m.err = err
		}
		return m, nil
	case homeLaunchpad:
		m.screen = screenLaunchpad
		m.cursor = 0
		m.err = nil
		return m, nil
	case homeQuit:
		return m, tea.Quit
	}
	return m, nil
}

// hasErrorCode reports whether err or any wrapped *errpkg.Error has the code.
func hasErrorCode(err error, code string) bool {
	for err != nil {
		var e *errpkg.Error
		if !errors.As(err, &e) {
			return false
		}
		if e.Code == code {
			return true
		}
		err = e.Unwrap()
	}
	return false
}

// launchSelected runs the selected launchpad entry. TUI-mode entries set
// pendingLaunch and quit so the outer RunLoop can spawn camp/fest on the real
// TTY; capture-mode entries (oneshot/stream) run inside the hub with piped
// output so their text is not lost when the hub repaints.
func (m model) launchSelected() (tea.Model, tea.Cmd) {
	if len(m.launchEntries) == 0 {
		m.err = errpkg.New("E_LAUNCH_EMPTY", "no launchpad entries")
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.launchEntries) {
		return m, nil
	}
	entry := m.launchEntries[m.cursor]
	// Preflight resolve so missing tools show an in-hub error instead of a black screen.
	if _, err := launch.Resolve(m.ctx, entry.Spec.Tool); err != nil {
		m.err = err
		return m, nil
	}
	if entry.Mode.IsCapture() {
		return m.startCapture(entry)
	}
	cp := entry.Spec
	m.pendingLaunch = &cp
	return m, tea.Quit
}

// selfToolName is the managed name of the hub binary itself.
const selfToolName = "festival"

// restartHub re-execs into the newly-updated festival binary with the same
// arguments this session started with. It only runs when the user presses r
// on a result screen offering the restart, never on its own: the update that
// replaced the binary does not quit the process by itself.
func (m model) restartHub() (tea.Model, tea.Cmd) {
	if _, err := launch.Resolve(m.ctx, selfToolName); err != nil {
		m.err = err
		return m, nil
	}
	spec := launch.Spec{Tool: selfToolName, Args: append([]string{}, os.Args[1:]...)}
	m.pendingLaunch = &spec
	return m, tea.Quit
}

// startCapture spawns a capture-mode child and switches to the output screen.
func (m model) startCapture(entry launch.Entry) (tea.Model, tea.Cmd) {
	cs, err := launch.StartCapture(m.ctx, entry.Spec)
	if err != nil {
		m.err = err
		return m, nil
	}
	m.capture = cs
	m.captureTitle = entry.Spec.Title
	if m.captureTitle == "" {
		m.captureTitle = entry.Spec.Tool
	}
	m.captureMode = entry.Mode
	m.captureRes = nil
	m.captureOut = nil
	m.captureVP = viewport.New(captureVPWidth(m.width), captureVPHeight(m.height))
	m.err = nil
	m.screen = screenChildOutput
	return m, captureNext(cs)
}

// captureNext waits for the next output chunk or the child's exit.
func captureNext(cs *launch.Capture) tea.Cmd {
	return func() tea.Msg {
		chunk, res := cs.Next()
		if res != nil {
			return childExitMsg(*res)
		}
		return childChunkMsg(chunk)
	}
}

// closeChildOutput leaves the capture screen: a running child is stopped
// first (its exit arrives via captureNext); once exited, return to launchpad.
func (m model) closeChildOutput() (tea.Model, tea.Cmd) {
	if m.capture != nil {
		m.capture.Stop()
		return m, nil
	}
	m.captureRes = nil
	m.captureOut = nil
	m.screen = screenLaunchpad
	return m, nil
}

// captureVPWidth/Height keep the output viewport inside the hub chrome
// (header, title line, footer).
func captureVPWidth(w int) int {
	return max(20, w-2)
}

func captureVPHeight(h int) int {
	return max(5, h-6)
}

func (m model) startInstall(allowUnverified bool) (tea.Model, tea.Cmd) {
	ch := m.channels[m.channelIdx]
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "resolve", Percent: 0, Message: "starting install"})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runInstall(ctx, ch, allowUnverified, m.installForce, ps), waitProgress(ps))
}

func runInstall(ctx context.Context, channel string, allowUnverified, force bool, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, err := app.InstallFestival(ctx, app.InstallOptions{
			Channel:  channel,
			Verify:   tuiVerifyOptions(ps, allowUnverified),
			Progress: ps.report,
			Force:    force,
		})
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "install-unverified", cause: err}
			}
			return opFailed(ps, "Install failed", err, "")
		}
		body := fmt.Sprintf("installed %s %s (%s)\n", res.Package, res.Version, res.Channel)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{stream: ps, title: "Install complete", body: body, success: true}
	}
}

func (m model) startUpdate(allowUnverified bool) (tea.Model, tea.Cmd) {
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "checking updates"})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runUpdate(ctx, allowUnverified, ps), waitProgress(ps))
}

func runUpdate(ctx context.Context, allowUnverified bool, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, warning, err := app.UpdateFestival(ctx, app.UpdateOptions{
			Verify:   tuiVerifyOptions(ps, allowUnverified),
			Progress: ps.report,
		})
		if err != nil && !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
			return consentNeededMsg{action: "update-unverified", cause: err}
		}
		// Package origin may still arrive as Action=package plus
		// E_INSTALL_PACKAGE_CHANNEL from older hubs. That is guidance,
		// not an update failure.
		if res.Action == "package" || hasErrorCode(err, "E_INSTALL_PACKAGE_CHANNEL") {
			if res.Action == "" {
				res.Action = "package"
			}
			if warning == "" && err != nil {
				warning = app.FriendlyMessage(err)
			}
			if spec, ok := packageUpgradeSpec(res); ok {
				return packageUpgradeMsg{spec: spec}
			}
			return updateOpDoneMsg(ps, res, warning)
		}
		if err != nil {
			return opFailed(ps, "Update failed", err, "")
		}
		return updateOpDoneMsg(ps, res, warning)
	}
}

// updateOpDoneMsg builds the result-screen message for a completed update,
// including the restart offer when the update replaced the running hub.
func updateOpDoneMsg(ps *progressStream, res app.UpdateResult, warning string) opDoneMsg {
	body := fmt.Sprintf("action: %s\nversion: %s\n", res.Action, res.Version)
	if res.From != "" {
		body += "from: " + res.From + "\n"
	}
	if warning != "" {
		body += "\n" + warning + "\n"
	}
	if res.SelfReplaced {
		body += "\nfestival was updated to " + res.Version + "; restart to use the new version\n"
	}
	ok := res.Action == "upgraded" || res.Action == "current" || res.Action == "package"
	title := "Update"
	switch res.Action {
	case "upgraded":
		title = "Updated"
	case "current":
		title = "Already current"
	case "package":
		title, body = packageUpdateResultView(res, warning)
		ok = true
	case "unmanaged":
		title = "Unmanaged install"
		ok = false
	case "absent":
		title = "Not installed"
		ok = false
	}
	return opDoneMsg{stream: ps, title: title, body: body, success: ok, restart: res.SelfReplaced}
}

func packageUpgradeSpec(res app.UpdateResult) (launch.Spec, bool) {
	if !app.PackageUpgradeAvailable(res) {
		return launch.Spec{}, false
	}
	tool, args, ok := app.ParseUpgradeArgv(res.Upgrade)
	if !ok {
		return launch.Spec{}, false
	}
	if _, err := exec.LookPath(tool); err != nil {
		return launch.Spec{}, false
	}
	return launch.Spec{Tool: tool, Args: args, Title: res.Upgrade, ReplaceHub: true}, true
}

func packageUpdateResultView(res app.UpdateResult, warning string) (title, body string) {
	title = "Package install"
	upgrade := res.Upgrade
	if upgrade == "" {
		upgrade = upgradeFromText(warning)
	}
	var b strings.Builder
	switch {
	case res.Latest != "" && res.Version != "" && installer.VersionLess(res.Version, res.Latest):
		title = "Update available"
		fmt.Fprintf(&b, "Festival %s is installed via the package manager.\n%s is available.\nThis upgrades camp, fest, and this hub.\n", res.Version, res.Latest)
	case res.Latest != "" && res.Version != "":
		title = "Already current"
		fmt.Fprintf(&b, "Festival %s is already current.\nIt is installed via the package manager.\n", res.Version)
	case res.Version != "":
		fmt.Fprintf(&b, "Festival %s is installed via the package manager.\n", res.Version)
	default:
		b.WriteString("Festival is installed via the package manager.\n")
	}
	if upgrade != "" {
		label := "Upgrade with:"
		if title == "Already current" {
			label = "Future upgrades:"
		}
		fmt.Fprintf(&b, "\n%s\n  %s\n", label, upgrade)
	}
	b.WriteString("\nfestival update will not plant ~/.obey/installer.")
	return title, b.String()
}

func upgradeFromText(s string) string {
	_, rest, ok := strings.Cut(s, "upgrade with: ")
	if !ok {
		return ""
	}
	line, _, _ := strings.Cut(rest, "\n")
	return strings.TrimSpace(line)
}

func (m model) startUninstall(packageID string) (tea.Model, tea.Cmd) {
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "activate", Percent: 0.5, Message: "removing " + packageID})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runUninstall(ctx, packageID, ps), waitProgress(ps))
}

// opFailed builds the result-screen message for a failed operation. Every
// failure path goes through here so the body is always the friendly rendering:
// this screen is where a first-time install failure lands, and printing the
// error chain there put raw git output in front of the newest users.
func opFailed(ps *progressStream, title string, err error, note string) opDoneMsg {
	body := app.FriendlyMessage(err)
	if note != "" {
		body += "\n\n" + note
	}
	return opDoneMsg{stream: ps, title: title, body: body, err: err, success: false}
}

func runUninstall(ctx context.Context, packageID string, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, err := app.UninstallPackage(ctx, packageID)
		if err != nil {
			return opFailed(ps, "Uninstall failed", err, "")
		}
		body := res.Note
		if body == "" {
			body = "uninstalled " + res.Package + "\n"
			for _, f := range res.Removed {
				body += "  removed " + f + "\n"
			}
		}
		return opDoneMsg{stream: ps, title: "Uninstall complete", body: body, success: true}
	}
}

func (m model) installBrowseSelection(allowUnverified bool) (tea.Model, tea.Cmd) {
	if len(m.browseFlat) == 0 {
		return m, nil
	}
	entry := m.browseFlat[m.cursor]
	// Map package ID segment to camp-/fest- selector when possible.
	target := entry.ID
	if i := strings.LastIndex(entry.ID, "/"); i >= 0 {
		seg := entry.ID[i+1:]
		if _, _, ok := app.PluginHost(seg); ok {
			target = seg
		} else if entry.Class == "bundle" || entry.Class == "product" || strings.Contains(entry.ID, "festival") {
			target = "festival"
		} else {
			// try installing by plugin short name if looks like camp-X/fest-X
			target = seg
		}
	}
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "installing " + entry.ID})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runTargetInstall(ctx, target, entry.ID, allowUnverified, ps), waitProgress(ps))
}

func runTargetInstall(ctx context.Context, target, entryID string, allowUnverified bool, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, err := app.InstallTarget(ctx, target, app.InstallOptions{
			Channel:  "stable",
			Verify:   tuiVerifyOptions(ps, allowUnverified),
			Progress: ps.report,
		})
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "browse-install-unverified", cause: err}
			}
			return opFailed(ps, "Install failed", err, "(selected "+entryID+" as "+target+")")
		}
		body := fmt.Sprintf("installed %s %s\n", res.Package, res.Version)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{stream: ps, title: "Install complete", body: body, success: true}
	}
}

func (m model) submitMarketplaceAdd() (tea.Model, tea.Cmd) {
	url := strings.TrimSpace(m.addInput.Value())
	if url == "" {
		m.err = errpkg.New("E_MARKETPLACE_URL_EMPTY", "enter a git URL")
		return m, nil
	}
	m.marketMode = "list"
	ctx := m.ctx
	return m, func() tea.Msg {
		_, err := app.MarketplaceAdd(ctx, url, "", tuiVerifyOptions(nil, false))
		if err != nil {
			return marketMsg{err: err}
		}
		views, err := app.MarketplaceList(ctx, tuiVerifyOptions(nil, false))
		return marketMsg{views: views, err: err}
	}
}

func flattenBrowse(res app.BrowseResult) []app.BrowseEntry {
	seen := map[string]bool{}
	var out []app.BrowseEntry
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	return out
}
