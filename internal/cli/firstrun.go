package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// NoTerminalMessage is what a bare `festival` prints when there is no terminal
// to open the TUI in and the home is already set up.
const NoTerminalMessage = `festival: no interactive terminal detected.

Open a terminal and run:  festival
Or use a subcommand:      festival install festival
                          festival --help
`

// BareInvocation renders what `festival` with no arguments should print when it
// cannot open the TUI. On a first-run home it names the three next steps; on any
// other home it keeps the previous behavior of pointing at a terminal and then
// printing help, because a set-up user asking for bare festival wants the
// command list, not onboarding.
func BareInvocation(ctx context.Context, out, errOut io.Writer, help func() error) error {
	setup, err := app.ResolveSetupState(ctx)
	if err == nil && setup.IsFirstRun() {
		_, werr := fmt.Fprint(out, FirstRunGuidance(ShellFromEnv()))
		return werr
	}
	if _, werr := fmt.Fprint(errOut, NoTerminalMessage); werr != nil {
		return werr
	}
	return help()
}

// FirstRunGuidance is the three-step onboarding text, written for the shell the
// user is actually running so step 2 can be pasted as printed.
func FirstRunGuidance(shell string) string {
	var b strings.Builder
	b.WriteString("festival is not set up on this machine yet.\n\n")
	b.WriteString("  1. Install the suite:   festival install festival\n")
	b.WriteString("  2. Wire your PATH:      " + shellInitLineFor(shell) + "\n")
	b.WriteString("  3. Browse the catalog:  festival browse\n\n")
	b.WriteString("Run 'festival doctor' at any point to see what is still pending.\n")
	return b.String()
}

// shellInitLineFor is the copy-pasteable line for one shell. fish has no eval,
// so printing the zsh form to a fish user would hand them something that fails.
func shellInitLineFor(shell string) string {
	if shell == "fish" {
		return "festival shell-init fish | source"
	}
	return `eval "$(festival shell-init ` + shell + `)"`
}

// ShellFromEnv resolves the user's shell from $SHELL, which is the pragmatic
// source for this. Anything unset or unrecognized falls back to zsh, matching
// what shell-init defaults to. Supported shells are zsh, bash and fish.
func ShellFromEnv() string {
	name := filepath.Base(strings.TrimSpace(os.Getenv("SHELL")))
	switch name {
	case "zsh", "bash", "fish":
		return name
	default:
		return "zsh"
	}
}
