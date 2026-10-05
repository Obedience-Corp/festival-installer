package tui

import (
	"fmt"
	"path/filepath"
	"strings"
)

const resultFileLimit = 8

func installedFilesBody(files []string) string {
	if len(files) == 0 {
		return ""
	}
	var bins, rest []string
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) == "bin" {
			bins = append(bins, f)
		} else {
			rest = append(rest, f)
		}
	}
	var b strings.Builder
	for _, f := range bins {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	shown := rest
	if len(rest) > resultFileLimit {
		shown = rest[:resultFileLimit]
	}
	for _, f := range shown {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	if hidden := len(rest) - len(shown); hidden > 0 {
		fmt.Fprintf(&b, "  ... and %d more supporting files under %s\n", hidden, commonDir(rest))
	}
	return b.String()
}

func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	sep := string(filepath.Separator)
	common := strings.Split(filepath.Dir(paths[0]), sep)
	for _, p := range paths[1:] {
		parts := strings.Split(filepath.Dir(p), sep)
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	dir := strings.Join(common, sep)
	if dir == "" {
		return sep
	}
	return dir
}
