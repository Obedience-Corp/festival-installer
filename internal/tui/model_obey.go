package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

type obeyInventoryMsg struct {
	id  uint64
	res app.ListResult
	err error
}

func (m model) openObey() (tea.Model, tea.Cmd) {
	m.screen = screenObey
	m.err = nil
	m.obeyLoading = true
	m.obeyLoadErr = nil
	m.obeyLoadID++
	id, ctx := m.obeyLoadID, m.ctx
	return m, func() tea.Msg {
		res, err := app.ListInstalled(ctx)
		return obeyInventoryMsg{id: id, res: res, err: err}
	}
}

func (m model) obeyReceipt() (app.ListEntry, bool) {
	for _, p := range m.obeyInventory.Packages {
		if p.PackageID == app.ObeyPackageID {
			return p, true
		}
	}
	return app.ListEntry{}, false
}

func (m model) viewObey() string {
	s := m.styles
	lines := []string{s.Title.Render("Obey"), "Local agent sessions, in your terminal.", "", "Includes obey (agent daemon and chat) and ob (command-line client).", "The Festival suite is separate; you can add Obey at any time.", ""}
	compact := m.width < 60 || m.height < 24
	if compact {
		lines = []string{s.Title.Render("Obey"), "Agent sessions and chat. Includes obey + ob.", ""}
	}
	switch {
	case m.obeyLoading:
		lines = append(lines, "Reading installed packages...")
	case m.obeyLoadErr != nil:
		lines = append(lines, s.Warn.Render("Installation status unavailable."), ansi.Truncate(textsafe.Line(m.obeyLoadErr.Error()), max(20, m.width-4), "…"), "Press r to retry; installation is disabled.")
	default:
		if p, ok := m.obeyReceipt(); ok {
			lines = append(lines, s.OK.Render("Installed · "+textsafe.Line(p.Version)), "", s.Fire.Render("Enter to check for updates"))
		} else {
			lines = append(lines, "Not installed through Festival.", "", s.Fire.Render("Enter to install Obey · stable"))
		}
		if compact {
			lines = append(lines, "", "Sets up daemon and terminal integration.", "Active sessions keep running.", "Any required restart is deferred.", "Then open a new terminal: obey chat")
		} else {
			lines = append(lines, "", "Sets up the daemon and terminal integration.", "Existing running sessions stay active; any required restart is deferred.", "After installation, open a new terminal and run obey chat.")
		}
	}
	return lipgloss.NewStyle().Width(max(20, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m model) startObey(allowUnverified bool) (tea.Model, tea.Cmd) {
	if m.obeyLoading || m.obeyLoadErr != nil {
		return m, nil
	}
	_, installed := m.obeyReceipt()
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "resolve", Package: app.ObeyPackageID, Message: "checking Obey"})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(runObey(ctx, installed, allowUnverified, ps), waitProgress(ps))
}

func runObey(ctx context.Context, installed, allowUnverified bool, ps *progressStream) tea.Cmd {
	return func() tea.Msg {
		defer ps.close()
		var body string
		var err error
		if installed {
			var res app.UpdateResult
			var warning string
			res, warning, err = app.UpdateObey(ctx, app.UpdateOptions{Verify: tuiVerifyOptions(ps, allowUnverified), Progress: ps.report, NoRestart: true})
			body = fmt.Sprintf("Obey · %s\nVersion: %s\n", res.Action, res.Version)
			if warning != "" {
				body += "\n" + warning + "\n"
			}

			if err == nil && res.Action != "current" && res.Action != "upgraded" {
				return opDoneMsg{stream: ps, title: "Obey needs attention", body: body, success: false}
			}
		} else {
			var res app.InstallResult
			res, err = app.InstallObey(ctx, app.InstallOptions{Channel: "stable", Verify: tuiVerifyOptions(ps, allowUnverified), Progress: ps.report, NoRestart: true})
			body = fmt.Sprintf("Obey %s installed\n", res.Version) + installedFilesBody(res.Files)
			if note := app.ServiceNoteFor(res); note != "" {
				body += "\n" + note + "\n"
			}
			if note := app.IntegrationNote(res.Integrations); note != "" {
				body += "\n" + note + "\n"
			}
		}
		if err != nil {
			if !allowUnverified && hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
				return consentNeededMsg{action: "obey-unverified", cause: err}
			}
			return opFailed(ps, "Obey setup failed", err, "")
		}
		body += "\nOpen a new terminal, then run: obey chat\n"
		return opDoneMsg{stream: ps, title: "Obey", body: body, success: true}
	}
}
