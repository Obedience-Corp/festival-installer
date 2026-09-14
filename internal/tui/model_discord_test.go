package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func fakeBrowser(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range []string{"open", "xdg-open"} {
		if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestDiscordMenuOpensInviteAndReturnsHome(t *testing.T) {
	dir := fakeBrowser(t, "printf '%s' \"$1\" > \"$FESTIVAL_TEST_INVITE\"\n")
	destination := filepath.Join(dir, "invite")
	t.Setenv("FESTIVAL_TEST_INVITE", destination)
	m := newModel(Options{Version: "test"})
	m.screen = screenHome
	m.cursor = m.homeIndexOf(homeDiscord)
	if !strings.Contains(m.View(), "Join our Discord") {
		t.Fatal("home must advertise the Discord entry")
	}
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if cmd == nil || !m.busy || m.screen != screenDiscord {
		t.Fatal("selecting Discord must launch the opener asynchronously")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "https://discord.gg/Rt7dDY6VqD" {
		t.Fatalf("opened %q, err %v", got, err)
	}
	if m.busy || m.err != nil || !strings.Contains(m.View(), "Invite sent to your browser") {
		t.Fatalf("unexpected completion: busy=%v err=%v view=%s", m.busy, m.err, m.View())
	}
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(model).screen != screenHome {
		t.Fatal("enter should return to the home menu")
	}
}

func TestDiscordOpenerPlatformsAndFailures(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			fakeBrowser(t, "test \"$#\" = 1 && test \"$1\" = 'https://discord.gg/Rt7dDY6VqD'\n")
			if err := openDiscordInvite(context.Background(), goos); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := openDiscordInvite(ctx, goos); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation not preserved: %v", err)
			}
		})
	}
	for _, mode := range []string{"missing", "failed"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "missing" {
				t.Setenv("PATH", t.TempDir())
			} else {
				fakeBrowser(t, "exit 1\n")
			}
			m := newModel(Options{Version: "test"})
			next, cmd := m.joinDiscord()
			m = next.(model)
			next, _ = m.Update(cmd())
			m = next.(model)
			if m.err == nil || m.busy || !strings.Contains(m.View(), "Could not open a browser") || !strings.Contains(m.View(), discordInviteURL) {
				t.Fatalf("failure must offer the manual invite: %s", m.View())
			}
		})
	}
	if err := openDiscordInvite(context.Background(), "unsupported"); err == nil {
		t.Fatal("unsupported platform must return an error")
	}
}

func TestLeavingDiscordCancelsOpenerAndIgnoresLateResult(t *testing.T) {
	fakeBrowser(t, "exit 0\n")
	m := newModel(Options{Version: "test"})
	next, cmd := m.joinDiscord()
	m = next.(model)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	msg := cmd().(discordOpenedMsg)
	if !errors.Is(msg.ctx.Err(), context.Canceled) {
		t.Fatal("leaving must cancel the pending opener")
	}
	// Even when the user opens Discord again, the old response cannot replace it.
	next, _ = m.joinDiscord()
	m = next.(model)
	t.Cleanup(m.opCancel)
	next, _ = m.Update(msg)
	m = next.(model)
	if m.screen != screenDiscord || !m.busy || m.err != nil {
		t.Fatal("late response changed the new Discord screen")
	}
}
