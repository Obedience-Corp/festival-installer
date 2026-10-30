package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type updatesMsg struct {
	generation uint64
	result     app.UpdatesResult
	err        error
}

func (m model) loadUpdates() tea.Cmd {
	ctx, generation := m.ctx, m.updatesGeneration
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		result, err := app.CheckUpdates(ctx, tuiVerifyOptions(nil, false))
		return updatesMsg{generation: generation, result: result, err: err}
	}
}

func (m model) refreshUpdates() (model, tea.Cmd) {
	if m.updatesChecking {
		return m, nil
	}
	m.updatesChecking = true
	m.updatesGeneration++
	return m, m.loadUpdates()
}

func (m model) openUpdates() (tea.Model, tea.Cmd) {
	m.screen, m.cursor, m.err = screenUpdates, 0, nil
	if !m.updatesChecking && (m.updates.CheckedAt.IsZero() || time.Since(m.updates.CheckedAt) > 5*time.Minute) {
		return m.refreshUpdates()
	}
	return m, nil
}

func (m model) updatesLabel() string {
	if m.updatesChecking {
		return "Updates · checking"
	}
	if m.updatesErr != nil || m.updates.Failed() {
		return fmt.Sprintf("Updates · %d available, check failed", m.updates.Available())
	}
	if n := m.updates.Available(); n > 0 {
		return fmt.Sprintf("Updates · %d available", n)
	}
	return "Updates"
}

func (m model) viewUpdates() string {
	s := m.styles
	width := max(20, m.width-4)
	var lines []string
	switch {
	case m.updatesChecking:
		lines = append(lines, "Checking installed packages… Previous results are shown below.")
	case m.updatesErr != nil:
		lines = append(lines, "Could not check updates: "+textsafe.Line(m.updatesErr.Error()))
	case m.updates.CheckedAt.IsZero():
		lines = append(lines, "Updates have not been checked. Press r to check.")
	default:
		lines = append(lines, "Checked "+m.updates.CheckedAt.Local().Format("15:04:05")+" · "+fmt.Sprint(m.updates.Available())+" updates available")
	}
	lines = append(lines, "Update all preserves installed channels and running Obey sessions.")
	rows := []string{fmt.Sprintf("Update all (%d available)", m.updates.Available())}
	for _, p := range m.updates.Packages {
		name := p.Package
		if width < 68 {
			if _, short, ok := strings.Cut(name, "/"); ok {
				name = short
			}
		}
		line := fmt.Sprintf("%s  %s", name, p.Installed)
		if p.Latest != "" {
			line += " → " + p.Latest
		}
		rows = append(rows, line+"  ["+p.Status+"]")
	}
	visible := max(1, m.height-11)
	start := max(0, m.cursor-visible+1)
	for i := start; i < len(rows) && i < start+visible; i++ {
		line := ansi.Truncate(textsafe.Line(rows[i]), width-4, "…")
		if i == m.cursor {
			line = s.Selected.Render("› " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(rows) > visible {
		lines = append(lines, fmt.Sprintf("%d/%d · ↑↓ scroll", m.cursor+1, len(rows)))
	}
	if m.cursor > 0 && m.cursor <= len(m.updates.Packages) {
		p := m.updates.Packages[m.cursor-1]
		detail := p.Package + " · " + p.Channel + "\n" + p.Source
		if p.Detail != "" {
			detail += "\n" + textsafe.Line(p.Detail)
		}
		lines = append(lines, detail)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) selectUpdate() (tea.Model, tea.Cmd) {
	if m.updatesChecking || m.updatesErr != nil {
		return m, nil
	}
	if m.cursor == 0 {
		if m.updates.Available() == 0 {
			return m, nil
		}
		return m.startPackageUpdates("")
	}
	if m.cursor > len(m.updates.Packages) {
		return m, nil
	}
	p := m.updates.Packages[m.cursor-1]
	if p.Status == app.UpdateExternal && p.Package == app.FestivalPackageID && m.status.Action == "package" {
		return m.startUpdate(false)
	}
	if p.Status != app.UpdateAvailable {
		return m, nil
	}
	return m.startPackageUpdates(p.Package)
}

func (m model) startPackageUpdates(packageID string) (tea.Model, tea.Cmd) {
	m.updatesGeneration++ // Discard discovery that began before this operation.
	m.updatesChecking = false
	m, ps := m.beginProgress(app.ProgressEvent{Stage: "resolve", Message: "checking selected updates", Percent: 0.1})
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, tea.Batch(func() tea.Msg {
		defer ps.close()
		opts := app.UpdateOptions{Verify: tuiVerifyOptions(ps, false), Progress: ps.report, NoRestart: true}
		if packageID != "" {
			result, warning, err := app.UpdateInstalled(ctx, packageID, opts)
			if err != nil {
				return opFailed(ps, "Update failed", err, "")
			}
			return updateOpDoneMsg(ps, result, warning)
		}
		result, err := app.UpdateAll(ctx, opts)
		if err != nil {
			return opFailed(ps, "Updates failed", err, "")
		}
		var body strings.Builder
		restart := false
		for _, p := range result.Packages {
			fmt.Fprintf(&body, "%s: %s", p.Package, p.Action)
			if p.From != "" {
				fmt.Fprintf(&body, " %s → %s", p.From, p.Version)
			}
			body.WriteByte('\n')
			if p.Warning != "" {
				body.WriteString(p.Warning + "\n")
			}
			if p.Error != "" {
				body.WriteString(p.Error + "\n")
			}
			restart = restart || p.SelfReplaced
		}
		title := "Updates complete"
		if result.Failed() {
			title = "Some updates failed"
			body.WriteString("Successful updates were retained. Refresh and retry the remaining packages.\n")
		}
		if restart {
			body.WriteString("Restart Festival to use the updated installer.\n")
		}
		return opDoneMsg{stream: ps, title: title, body: body.String(), success: !result.Failed(), restart: restart, warningsAfter: true}
	}, waitProgress(ps))
}

func (m model) updateAnnotation(id string) string {
	for _, p := range m.updates.Packages {
		if p.Package == id {
			if p.Status == app.UpdateAvailable {
				return " · " + p.Installed + " → " + p.Latest + " available"
			}
			if p.Status == app.UpdateCheckFailed {
				return " · update check failed"
			}
			return " · installed " + p.Installed
		}
	}
	return ""
}
