package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenHome:
		return m.openHomeItem()
	case screenInstall:
		// Try strict first; prompt only when VER-01 refuses unsigned content.
		return m.startInstall(false)
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
		m.confirmMsg = "Uninstall " + pkg.PackageID + "?"
		m.confirmYes = false
		m.confirmAct = "uninstall"
		m.confirmArg = pkg.PackageID
		m.screen = screenConfirm
		return m, nil
	case screenConfirm:
		if !m.confirmYes {
			m.screen = screenHome
			m.err = nil
			return m, nil
		}
		switch m.confirmAct {
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
				_, err := app.MarketplaceRefresh(ctx, "")
				if err != nil {
					return marketMsg{err: err}
				}
				views, err := app.MarketplaceList(ctx)
				return marketMsg{views: views, err: err}
			})
		}
		// remove selected marketplace? use 'd' - for enter, refresh single
		name := m.markets[m.cursor].Name
		return m, tea.Batch(func() tea.Msg {
			_, err := app.MarketplaceRefresh(ctx, name)
			if err != nil {
				return marketMsg{err: err}
			}
			views, err := app.MarketplaceList(ctx)
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
	switch m.cursor {
	case 0:
		m.screen = screenInstall
		m.channelIdx = 0
		return m, nil
	case 1:
		m.screen = screenUpdate
		return m.startUpdate(false)
	case 2:
		m.screen = screenList
		return m, m.loadList()
	case 3:
		m.screen = screenBrowse
		m.productF, m.kindF = "", ""
		return m, m.loadBrowse("", "")
	case 4:
		m.screen = screenUninstall
		return m, m.loadList()
	case 5:
		m.screen = screenMarketplace
		m.marketMode = "list"
		return m, m.loadMarkets()
	case 6:
		m.screen = screenDoctor
		return m, m.loadDoctor()
	case 7:
		m.screen = screenShell
		bin, on, err := app.ManagedBinOnPath(m.ctx)
		m.shellBin = bin
		m.shellOnPath = on
		if err != nil {
			m.err = err
		}
		snip, _ := app.ShellInit(m.ctx, "zsh")
		m.shellSnippet = snip
		return m, nil
	case 8:
		m.screen = screenLaunchpad
		m.cursor = 0
		m.err = nil
		return m, nil
	case 9:
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
	return m, tea.Batch(runInstall(ctx, ch, allowUnverified, ps), waitProgress(ps))
}

func runInstall(ctx context.Context, channel string, allowUnverified bool, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, err := app.InstallFestival(ctx, app.InstallOptions{
			Channel:  channel,
			Verify:   source.DefaultVerifyOptions(nil, allowUnverified),
			Progress: ps.report,
		})
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "install-unverified", cause: err}
			}
			return opDoneMsg{stream: ps, title: "Install failed", body: err.Error(), err: err, success: false}
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
			Verify:   source.DefaultVerifyOptions(nil, allowUnverified),
			Progress: ps.report,
		})
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "update-unverified", cause: err}
			}
			return opDoneMsg{stream: ps, title: "Update failed", body: err.Error(), err: err, success: false}
		}
		body := fmt.Sprintf("action: %s\nversion: %s\n", res.Action, res.Version)
		if res.From != "" {
			body += "from: " + res.From + "\n"
		}
		if warning != "" {
			body += "\n" + warning + "\n"
		}
		ok := res.Action == "upgraded" || res.Action == "current"
		title := "Update"
		switch res.Action {
		case "upgraded":
			title = "Updated"
		case "current":
			title = "Already current"
		case "unmanaged":
			title = "Unmanaged install"
			ok = false
		case "absent":
			title = "Not installed"
			ok = false
		}
		return opDoneMsg{stream: ps, title: title, body: body, success: ok}
	}
}

func (m model) startUninstall(packageID string) (tea.Model, tea.Cmd) {
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "activate", Percent: 0.5, Message: "removing " + packageID})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runUninstall(ctx, packageID, ps), waitProgress(ps))
}

func runUninstall(ctx context.Context, packageID string, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		res, err := app.UninstallPackage(ctx, packageID)
		if err != nil {
			return opDoneMsg{stream: ps, title: "Uninstall failed", body: err.Error(), err: err, success: false}
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
			Verify:   source.DefaultVerifyOptions(nil, allowUnverified),
			Progress: ps.report,
		})
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "browse-install-unverified", cause: err}
			}
			return opDoneMsg{stream: ps, title: "Install failed", body: err.Error() + "\n\n(selected " + entryID + " as " + target + ")", err: err, success: false}
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
		_, err := app.MarketplaceAdd(ctx, url, "")
		if err != nil {
			return marketMsg{err: err}
		}
		views, err := app.MarketplaceList(ctx)
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
