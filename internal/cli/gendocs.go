package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// NewGendocsCommand builds the hidden gendocs command that renders the
// festival hub's CLI reference for the distribution repo's docs site.
func NewGendocsCommand() *cobra.Command {
	var output string
	var format string
	cmd := &cobra.Command{
		Use:    "gendocs",
		Short:  "Generate CLI reference documentation",
		Long:   "Generate markdown or YAML reference documentation for all festival commands.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return errpkg.Wrap("E_GENDOCS_CTX", err, "context cancelled")
			}
			return runGendocs(cmd.Root(), output, format)
		},
	}
	cmd.Flags().StringVar(&output, "output", "docs", "output directory")
	cmd.Flags().StringVar(&format, "format", "markdown", "output format: markdown or yaml")
	return cmd
}

func runGendocs(root *cobra.Command, output, format string) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return errpkg.Wrap("E_GENDOCS_OUTPUT_DIR", err, "create output dir")
	}

	if err := removeGeneratedDocs(output, "festival"); err != nil {
		return errpkg.Wrap("E_GENDOCS_CLEAN", err, "clean generated docs")
	}

	disableAutoGenTag(root)

	switch format {
	case "markdown":
		if err := doc.GenMarkdownTree(root, output); err != nil {
			return errpkg.Wrap("E_GENDOCS_MARKDOWN", err, "generate markdown")
		}
		if err := trimTrailingBlankLines(output); err != nil {
			return errpkg.Wrap("E_GENDOCS_TRIM", err, "trim trailing blank lines")
		}
	case "yaml":
		if err := doc.GenYamlTree(root, output); err != nil {
			return errpkg.Wrap("E_GENDOCS_YAML", err, "generate yaml")
		}
	default:
		return errpkg.New("E_GENDOCS_FORMAT", fmt.Sprintf("unknown format %q (use markdown or yaml)", format))
	}

	return nil
}

func disableAutoGenTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}

// trimTrailingBlankLines collapses trailing blank lines on every .md file in
// dir to a single newline. cobra/doc emits "...content\n\n", which trips
// `git diff --check` with "new blank line at EOF".
func trimTrailingBlankLines(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		trimmed := strings.TrimRight(string(data), "\n") + "\n"
		if trimmed == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(trimmed), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func removeGeneratedDocs(dir, name string) error {
	patterns := []string{
		filepath.Join(dir, name+"_*.md"),
		filepath.Join(dir, name+"_*.yaml"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		for _, file := range files {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
