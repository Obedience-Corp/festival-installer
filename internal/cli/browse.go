package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/source"
)

const noHostRuntime = "(unspecified)"

type browseEntry struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Class        string   `json:"class"`
	Summary      string   `json:"summary,omitempty"`
	HostRuntimes []string `json:"host_runtimes"`
	Channels     []string `json:"channels"`
	Source       string   `json:"source"`
}

type browseGroup struct {
	HostRuntime string        `json:"host_runtime"`
	Packages    []browseEntry `json:"packages"`
}

type browseResult struct {
	Groups []browseGroup `json:"groups"`
}

func NewBrowseCommand() *cobra.Command {
	var asJSON bool
	var product, kind string
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Browse available packages across registered marketplaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgs, err := source.AllPackages(cmd.Context())
			if err != nil {
				return err
			}
			res := buildBrowseResult(pkgs, product, kind)
			if asJSON {
				return jsonout.Print(cmd.OutOrStdout(), res)
			}
			return renderBrowseTable(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().StringVar(&product, "product", "", "filter by host product (fest|camp|obey)")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by package class (plugin|tool|product|bundle)")
	return cmd
}

func matchKind(p source.PackageRef, kind string) bool {
	return kind == "" || p.Class == kind
}

func matchProduct(p source.PackageRef, product string) bool {
	if product == "" {
		return true
	}
	if runtimeMatches(p.HostRuntimes, product) {
		return true
	}
	for _, t := range p.Targets {
		if runtimeMatches([]string{t.Runtime}, product) {
			return true
		}
	}
	return false
}

func runtimeMatches(runtimes []string, product string) bool {
	for _, r := range runtimes {
		if r == product || strings.HasPrefix(r, product+"-") {
			return true
		}
	}
	return false
}

func buildBrowseResult(pkgs []source.BrowsePackage, product, kind string) browseResult {
	grouped := map[string][]browseEntry{}
	for _, bp := range pkgs {
		p := bp.Package
		if !matchKind(p, kind) || !matchProduct(p, product) {
			continue
		}
		entry := browseEntry{
			ID:           p.ID,
			DisplayName:  p.DisplayName,
			Class:        p.Class,
			Summary:      p.Summary,
			HostRuntimes: nonNilStrings(p.HostRuntimes),
			Channels:     nonNilStrings(p.Channels),
			Source:       bp.Source,
		}
		runtimes := p.HostRuntimes
		if len(runtimes) == 0 {
			runtimes = []string{noHostRuntime}
		}
		for _, hr := range runtimes {
			grouped[hr] = append(grouped[hr], entry)
		}
	}

	keys := make([]string, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := browseResult{Groups: []browseGroup{}}
	for _, k := range keys {
		entries := grouped[k]
		sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
		result.Groups = append(result.Groups, browseGroup{HostRuntime: k, Packages: entries})
	}
	return result
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func renderBrowseTable(out io.Writer, res browseResult) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "HOST RUNTIME\tID\tNAME\tCLASS\tSOURCE")
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", g.HostRuntime, p.ID, p.DisplayName, p.Class, p.Source)
		}
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render browse table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
