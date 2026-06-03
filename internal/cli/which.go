package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

type whichResult struct {
	Tool     string `json:"tool"`
	Path     string `json:"path,omitempty"`
	Managed  string `json:"managed,omitempty"`
	OnPath   bool   `json:"on_path"`
	Shadowed bool   `json:"shadowed"`
}

func NewWhichCommand() *cobra.Command {
	var asJSON, showAll bool
	cmd := &cobra.Command{
		Use:   "which <tool>",
		Short: "Resolve the real binary path for a suite tool",
		Long: "which resolves where a tool (camp, fest, obey, ...) actually runs from.\n\n" +
			"camp and fest install as shell functions for directory-changing navigation, so a\n" +
			"plain `which camp` prints the function. This resolves the binary on PATH (past the\n" +
			"function) and flags when a binary shadows the installer-managed one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := resolveWhich(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return jsonout.Print(out, res)
			}
			if showAll {
				return renderWhichTable(out, res)
			}
			loc := res.Path
			if loc == "" {
				loc = res.Managed
			}
			if _, err := fmt.Fprintln(out, loc); err != nil {
				return err
			}
			if res.Shadowed {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: managed %s at %s is shadowed by %s\n", res.Tool, res.Managed, res.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&showAll, "show-all", false, "show the active and managed locations")
	return cmd
}

func resolveWhich(ctx context.Context, tool string) (whichResult, error) {
	r := whichResult{Tool: tool}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return r, err
	}
	managed := filepath.Join(binDir, tool)
	switch fi, statErr := os.Stat(managed); {
	case statErr == nil:
		if !fi.IsDir() {
			r.Managed = managed
		}
	case !os.IsNotExist(statErr):
		return r, errpkg.Wrap("E_MANAGED_STAT", statErr, "stat managed binary "+managed)
	}
	if active, err := exec.LookPath(tool); err == nil {
		r.Path = active
		r.OnPath = true
	}
	if r.Path == "" && r.Managed == "" {
		return r, errpkg.New("E_TOOL_NOT_FOUND",
			"no "+tool+" binary found on PATH or in the managed bin directory")
	}
	if r.Path != "" && r.Managed != "" {
		r.Shadowed = !samePath(r.Path, r.Managed)
	}
	return r, nil
}

func renderWhichTable(out io.Writer, res whichResult) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tLOCATION\tSOURCE\tSTATUS")
	if res.Path != "" {
		status := "active"
		if res.Shadowed {
			status = "active (shadows managed)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.Tool, res.Path, "path", status)
	}
	if res.Managed != "" && !samePath(res.Managed, res.Path) {
		status := "managed"
		if res.Shadowed {
			status = "shadowed"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.Tool, res.Managed, "managed", status)
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render which table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra := resolvePath(a)
	rb := resolvePath(b)
	return ra == rb
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}
