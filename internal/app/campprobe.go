package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// campProbeTimeout bounds the camp query. The tour runs it on every load of the
// tour screen, so a camp that hangs must not hang the hub.
const campProbeTimeout = 5 * time.Second

// CampInventory is what the hub could learn by asking camp what camps exist.
// It deliberately has no error: the tour must render on a machine where camp is
// missing, older, or broken, and every one of those cases is "no camp yet"
// rather than a failure to report.
type CampInventory struct {
	// Installed is true when a camp binary was found.
	Installed bool
	// Answered is true when camp ran and returned a list this hub could read.
	Answered bool
	// Count is how many camps camp reported.
	Count int
}

// Any reports whether camp knows about at least one camp.
func (c CampInventory) Any() bool { return c.Answered && c.Count > 0 }

// ProbeCamps asks camp what camps are registered, using camp's own
// machine-readable listing rather than looking for a directory layout. Camp owns
// what a camp is, including where its registry lives, so guessing at the
// filesystem here would go stale the first time camp changes.
func ProbeCamps(ctx context.Context) CampInventory {
	if ctx.Err() != nil {
		return CampInventory{}
	}
	path, err := ResolveTool(ctx, "camp")
	if err != nil {
		return CampInventory{}
	}
	inv := CampInventory{Installed: true}

	runCtx, cancel := context.WithTimeout(ctx, campProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, path, "list", "--json") //nolint:gosec // path from ResolveTool, args fixed
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return inv
	}
	count, ok := parseCampList(out)
	if !ok {
		return inv
	}
	inv.Answered = true
	inv.Count = count
	return inv
}

// parseCampList counts the entries in camp's JSON listing. An empty body or a
// JSON null both mean zero camps, which is an answer; anything else this hub
// cannot read means no answer at all.
func parseCampList(out []byte) (int, bool) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return 0, true
	}
	var entries []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return 0, false
	}
	return len(entries), true
}
