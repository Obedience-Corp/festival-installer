package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const discordInviteURL = "https://discord.gg/Rt7dDY6VqD"

type discordOpenedMsg struct {
	ctx context.Context
	err error
}

func (m model) joinDiscord() (tea.Model, tea.Cmd) {
	m.screen = screenDiscord
	m.err = nil
	m.busy = true
	ctx, cancel := m.opContext()
	m.opCancel = cancel
	return m, func() tea.Msg {
		return discordOpenedMsg{ctx: ctx, err: openDiscordInvite(ctx, runtime.GOOS)}
	}
}

// Use the desktop URL handler, which can hand the HTTPS invite to Discord.
// No shell, installation, login, or server join is performed by the hub.
func openDiscordInvite(ctx context.Context, goos string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var tool string
	switch goos {
	case "darwin":
		tool = "open"
	case "linux":
		tool = "xdg-open"
	default:
		return fmt.Errorf("opening a browser is not supported on %s", goos)
	}
	if err := exec.CommandContext(ctx, tool, discordInviteURL).Run(); err != nil {
		return fmt.Errorf("open Discord invite: %w", err)
	}
	return nil
}

func (m model) viewDiscord() string {
	s := m.styles
	status := "Invite sent to your browser.\nContinue there to join Obedience Corp."
	if m.busy {
		status = "Opening the Discord invite in your browser..."
	} else if m.err != nil {
		status = "Could not open a browser.\nOpen this invite on a device with a browser:"
	}
	return s.Title.Render("Join our Discord") + "\n\n" +
		s.Normal.Render("Get help with Festival and share what you build.") + "\n\n" +
		s.Normal.Render(status) + "\n\n" + s.Normal.Render(discordInviteURL)
}
