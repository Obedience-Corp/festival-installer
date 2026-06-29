package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

const festivalPackageID = "obedience-corp/festival"

type installResult struct {
	Package string   `json:"package"`
	Version string   `json:"version"`
	Channel string   `json:"channel"`
	Source  string   `json:"source"`
	Files   []string `json:"files"`
}

func NewInstallCommand() *cobra.Command {
	var channel string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "install <festival|camp|fest>",
		Short: "Install the festival suite (camp + fest)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			if err := validateChannel(channel); err != nil {
				return err
			}
			var res installResult
			if host, name, ok := pluginHost(target); ok {
				r, err := installPlugin(cmd.Context(), host, name, channel)
				if err != nil {
					return err
				}
				res = r
			} else {
				switch target {
				case "festival", "camp", "fest":
				default:
					return errpkg.New("E_INSTALL_TARGET", "unknown install target "+target+" (expected festival, camp, fest, or a camp-*/fest-* plugin)")
				}
				if target == "camp" || target == "fest" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "installing the festival suite (camp + fest); camp and fest are not published independently")
				}
				r, err := installFestival(cmd.Context(), channel)
				if err != nil {
					return err
				}
				res = r
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "install", res, nil)
			}
			return renderInstallResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel (stable|rc|dev)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func validateChannel(c string) error {
	switch c {
	case "stable", "rc", "dev":
		return nil
	default:
		return errpkg.New("E_INSTALL_CHANNEL", "invalid channel "+c+" (expected stable, rc, or dev)")
	}
}

func installFestival(ctx context.Context, channel string) (installResult, error) {
	if err := ctx.Err(); err != nil {
		return installResult{}, errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled")
	}

	home, err := state.Home(ctx)
	if err != nil {
		return installResult{}, err
	}
	cfg, err := state.LoadConfig(ctx, home)
	if err != nil {
		return installResult{}, err
	}
	sourceName := cfg.Marketplaces.Default

	manifest, err := source.LoadPackageManifest(ctx, sourceName, festivalPackageID)
	if err != nil {
		return installResult{}, err
	}

	rel, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return installResult{}, err
	}
	art, err := installer.SelectArtifact(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return installResult{}, err
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		return installResult{}, err
	}

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return installResult{}, err
	}
	defer func() { _ = db.Close(ctx) }()

	tx, err := installer.Begin(ctx, db.Raw(), home)
	if err != nil {
		return installResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stageDir := tx.StagingDir()
	archivePath, err := artifacts.NewDownloader().Download(ctx, art.URL, stageDir)
	if err != nil {
		return installResult{}, err
	}
	if err := artifacts.VerifySHA256(ctx, archivePath, art.Sha256); err != nil {
		return installResult{}, err
	}
	extractDir := filepath.Join(stageDir, "extracted")
	if err := artifacts.ExtractTarGz(ctx, archivePath, extractDir); err != nil {
		return installResult{}, err
	}

	var files []string
	for _, entry := range rel.Install.Entries {
		if entry.Kind != "binary" {
			continue
		}
		srcName := entry.Source
		destName := entry.ExecutableName
		if destName == "" {
			destName = srcName
		}
		if err := shared.ValidateSegment(destName); err != nil {
			return installResult{}, err
		}
		stagedBin, err := artifacts.SafeJoin(extractDir, filepath.FromSlash(srcName))
		if err != nil {
			return installResult{}, err
		}
		hash, err := artifacts.SHA256(ctx, stagedBin)
		if err != nil {
			return installResult{}, err
		}
		dst := filepath.Join(binDir, destName)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: stagedBin, DestPath: dst, Sha256: hash, Mode: 0o755}); err != nil {
			return installResult{}, err
		}
		files = append(files, dst)
	}

	if _, err := tx.Commit(ctx, installer.ReceiptInfo{
		PackageID:   festivalPackageID,
		Version:     rel.Version,
		Channel:     channel,
		Source:      sourceName,
		ManifestURL: art.URL,
	}); err != nil {
		return installResult{}, err
	}

	return installResult{
		Package: festivalPackageID,
		Version: rel.Version,
		Channel: channel,
		Source:  sourceName,
		Files:   files,
	}, nil
}

func renderInstallResult(w io.Writer, res installResult) error {
	if _, err := fmt.Fprintf(w, "installed %s %s (%s) from %s\n", res.Package, res.Version, res.Channel, res.Source); err != nil {
		return err
	}
	for _, f := range res.Files {
		if _, err := fmt.Fprintf(w, "  %s\n", f); err != nil {
			return err
		}
	}
	return nil
}
