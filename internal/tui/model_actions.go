package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/obey-installer/internal/app"
	"github.com/Obedience-Corp/obey-installer/internal/source"
)

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenHome:
		return m.openHomeItem()
	case screenInstall:
		return m.startInstall()
	case screenUpdate:
		return m.startUpdate()
	case screenList:
		return m, nil
	case screenBrowse:
		return m.installBrowseSelection()
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
			return m, nil
		}
		if m.confirmAct == "uninstall" {
			return m.startUninstall(m.confirmArg)
		}
		return m, nil
	case screenMarketplace:
		if m.marketMode == "add" {
			return m.submitMarketplaceAdd()
		}
		if m.cursor >= len(m.markets) {
			// refresh all
			return m, tea.Batch(func() tea.Msg {
				_, err := app.MarketplaceRefresh(context.Background(), "")
				if err != nil {
					return marketMsg{err: err}
				}
				views, err := app.MarketplaceList(context.Background())
				return marketMsg{views: views, err: err}
			})
		}
		// remove selected marketplace? use 'd' - for enter, refresh single
		name := m.markets[m.cursor].Name
		return m, tea.Batch(func() tea.Msg {
			_, err := app.MarketplaceRefresh(context.Background(), name)
			if err != nil {
				return marketMsg{err: err}
			}
			views, err := app.MarketplaceList(context.Background())
			return marketMsg{views: views, err: err}
		})
	case screenResult:
		m.screen = screenHome
		m.err = nil
		return m, nil
	case screenDoctor, screenShell:
		m.screen = screenHome
		return m, nil
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
		return m.startUpdate()
	case 2:
		m.screen = screenList
		return m, loadList()
	case 3:
		m.screen = screenBrowse
		m.productF, m.kindF = "", ""
		return m, loadBrowse("", "")
	case 4:
		m.screen = screenUninstall
		return m, loadList()
	case 5:
		m.screen = screenMarketplace
		m.marketMode = "list"
		return m, loadMarkets()
	case 6:
		m.screen = screenDoctor
		return m, loadDoctor()
	case 7:
		m.screen = screenShell
		bin, on, err := app.ManagedBinOnPath(context.Background())
		m.shellBin = bin
		m.shellOnPath = on
		if err != nil {
			m.err = err
		}
		snip, _ := app.ShellInit(context.Background(), "zsh")
		m.shellSnippet = snip
		return m, nil
	case 8:
		return m, tea.Quit
	}
	return m, nil
}

func (m model) startInstall() (tea.Model, tea.Cmd) {
	ch := m.channels[m.channelIdx]
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0, Message: "starting install"}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, runInstall(ctx, ch)
}

func runInstall(ctx context.Context, channel string) tea.Cmd {
	return func() tea.Msg {
		prog := func(ev app.ProgressEvent) {
			// best-effort; bubbletea can't inject mid-cmd without program.Send
			// stage is still returned in final message body
			_ = ev
		}
		res, err := app.InstallFestival(ctx, app.InstallOptions{
			Channel:  channel,
			Verify:   source.DefaultVerifyOptions(nil, false),
			Progress: prog,
		})
		if err != nil {
			return opDoneMsg{title: "Install failed", body: err.Error(), err: err, success: false}
		}
		body := fmt.Sprintf("installed %s %s (%s)\n", res.Package, res.Version, res.Channel)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{title: "Install complete", body: body, success: true}
	}
}

func (m model) startUpdate() (tea.Model, tea.Cmd) {
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "checking updates"}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, func() tea.Msg {
		res, warning, err := app.UpdateFestival(ctx, app.UpdateOptions{
			Verify: source.DefaultVerifyOptions(nil, false),
		})
		if err != nil {
			return opDoneMsg{title: "Update failed", body: err.Error(), err: err, success: false}
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
		return opDoneMsg{title: title, body: body, success: ok}
	}
}

func (m model) startUninstall(packageID string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "activate", Percent: 0.5, Message: "removing " + packageID}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, func() tea.Msg {
		res, err := app.UninstallPackage(ctx, packageID)
		if err != nil {
			return opDoneMsg{title: "Uninstall failed", body: err.Error(), err: err, success: false}
		}
		body := res.Note
		if body == "" {
			body = "uninstalled " + res.Package + "\n"
			for _, f := range res.Removed {
				body += "  removed " + f + "\n"
			}
		}
		return opDoneMsg{title: "Uninstall complete", body: body, success: true}
	}
}

func (m model) installBrowseSelection() (tea.Model, tea.Cmd) {
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
	m.busy = true
	m.screen = screenProgress
	m.progress = app.ProgressEvent{Stage: "resolve", Percent: 0.1, Message: "installing " + entry.ID}
	ctx, cancel := context.WithCancel(context.Background())
	m.opCancel = cancel
	return m, func() tea.Msg {
		res, err := app.InstallTarget(ctx, target, app.InstallOptions{
			Channel: "stable",
			Verify:  source.DefaultVerifyOptions(nil, false),
		})
		if err != nil {
			return opDoneMsg{title: "Install failed", body: err.Error() + "\n\n(selected " + entry.ID + " as " + target + ")", err: err, success: false}
		}
		body := fmt.Sprintf("installed %s %s\n", res.Package, res.Version)
		for _, f := range res.Files {
			body += "  " + f + "\n"
		}
		return opDoneMsg{title: "Install complete", body: body, success: true}
	}
}

func (m model) submitMarketplaceAdd() (tea.Model, tea.Cmd) {
	url := strings.TrimSpace(m.addInput.Value())
	if url == "" {
		m.err = fmt.Errorf("enter a git URL")
		return m, nil
	}
	m.marketMode = "list"
	return m, func() tea.Msg {
		_, err := app.MarketplaceAdd(context.Background(), url, "")
		if err != nil {
			return marketMsg{err: err}
		}
		views, err := app.MarketplaceList(context.Background())
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
