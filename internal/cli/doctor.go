package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

type DoctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type DoctorData struct {
	Checks []DoctorCheck `json:"checks"`
}

func NewDoctorCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose installer state (PATH, sources, receipts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := runDoctor(cmd.Context())
			if asJSON {
				if err := jsonout.Success(cmd.OutOrStdout(), "doctor", DoctorData{Checks: checks}, []string{}); err != nil {
					return err
				}
			} else if err := renderDoctorTable(cmd.OutOrStdout(), checks); err != nil {
				return err
			}
			for _, c := range checks {
				if c.Status == "fail" {
					return errpkg.New("E_DOCTOR_FAIL", "one or more doctor checks failed")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func runDoctor(ctx context.Context) []DoctorCheck {
	return []DoctorCheck{
		checkManagedBinOnPath(ctx),
		checkSourcesReachable(ctx),
		checkReceiptsIntegrity(ctx),
		checkPathShadowing(ctx),
	}
}

func checkManagedBinOnPath(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "managed_bin_on_path"}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	if dirOnPath(binDir) {
		c.Status = "ok"
		c.Message = "managed bin dir is on PATH: " + binDir
	} else {
		c.Status = "fail"
		c.Message = "managed bin dir is not on PATH: " + binDir + " (run: eval \"$(obey-installer shell-init zsh)\")"
	}
	return c
}

func checkSourcesReachable(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "sources_reachable"}
	views, err := source.ListMarketplaces(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	var broken []string
	for _, v := range views {
		if v.Err != "" {
			broken = append(broken, v.Name)
		}
	}
	switch {
	case len(views) == 0:
		c.Status = "warn"
		c.Message = "no marketplaces registered"
	case len(broken) > 0:
		c.Status = "fail"
		c.Message = "unreachable sources: " + strings.Join(broken, ", ")
	default:
		c.Status = "ok"
		c.Message = fmt.Sprintf("%d source(s) reachable", len(views))
	}
	return c
}

func checkReceiptsIntegrity(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "receipts_integrity"}
	home, err := state.Home(ctx)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	defer func() { _ = db.Close(ctx) }()

	recs, err := receipts.List(ctx, db.Raw(), receipts.Filter{})
	if err != nil {
		c.Status = "fail"
		c.Message = err.Error()
		return c
	}
	var problems []string
	for _, r := range recs {
		for _, f := range r.OwnedFiles {
			if f.Hash == "" {
				continue
			}
			if err := artifacts.VerifySHA256(ctx, f.Path, f.Hash); err != nil {
				problems = append(problems, r.PackageID+":"+f.Path)
			}
		}
	}
	switch {
	case len(recs) == 0:
		c.Status = "ok"
		c.Message = "no receipts"
	case len(problems) > 0:
		c.Status = "fail"
		c.Message = "orphan or mismatched receipt files: " + strings.Join(problems, ", ")
	default:
		c.Status = "ok"
		c.Message = fmt.Sprintf("%d receipt(s) verified", len(recs))
	}
	return c
}

func checkPathShadowing(ctx context.Context) DoctorCheck {
	c := DoctorCheck{ID: "path_shadowing", Status: "ok", Message: "no managed binary is shadowed"}
	var shadowed []string
	for _, tool := range []string{"camp", "fest"} {
		res, err := resolveWhich(ctx, tool)
		if err != nil {
			continue
		}
		if res.Shadowed {
			shadowed = append(shadowed, tool)
		}
	}
	if len(shadowed) > 0 {
		c.Status = "warn"
		c.Message = "managed binaries shadowed on PATH: " + strings.Join(shadowed, ", ")
	}
	return c
}

func renderDoctorTable(out io.Writer, checks []DoctorCheck) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, c.Status, c.Message)
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render doctor table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
