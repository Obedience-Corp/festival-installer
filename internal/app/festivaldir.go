package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// festivalProbeTimeout bounds the fest query. The tour runs it whenever step 4
// is chosen, so a fest that hangs must not hang the hub.
const festivalProbeTimeout = 5 * time.Second

// Error codes ResolveFestivalDir reports. Callers branch on these rather than
// on message text, because the difference between "this camp has no festival"
// and "this is not a camp" changes what the hub offers to do next.
const (
	// CodeNoCampRoot means there is no camp to look inside. Creating a festival
	// is not the fix for this; creating a camp is.
	CodeNoCampRoot = "E_FESTIVAL_NO_CAMP"
	// CodeNoFestival means the camp is real and holds no festival yet.
	CodeNoFestival = "E_FESTIVAL_NONE"
	// CodeFestivalList means fest was asked and its answer could not be used.
	CodeFestivalList = "E_FESTIVAL_LIST"
)

// festivalStatusOrder is the order the hub prefers when a camp holds festivals
// in more than one state: something already running, then something prepared,
// then something still being planned.
//
// parked and ritual are deliberately left out even though fest lists them. A
// parked festival was set aside on purpose, and a ritual is recurring
// machinery rather than the piece of work a new user is here to start.

// ResolveFestivalDir asks fest which festivals campRoot holds and returns the
// directory of the first one, preferring active, then ready, then planning.
//
// It exists because fest next only works inside a festival directory. Running
// it at the camp root fails with "not inside a festival", so the hub has to
// pick the directory itself rather than hand fest a camp and hope.
func ResolveFestivalDir(ctx context.Context, campRoot string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errpkg.Wrap("E_FESTIVAL_CTX", err, "context cancelled before resolving a festival")
	}
	campRoot = strings.TrimSpace(campRoot)
	if campRoot == "" {
		return "", errpkg.New(CodeNoCampRoot, "no camp here to look for a festival in")
	}
	path, err := ResolveTool(ctx, "fest")
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, festivalProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, path, "list", "--json") //nolint:gosec // path from ResolveTool, args fixed
	cmd.Dir = campRoot
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return "", errpkg.Wrap(CodeFestivalList, err, "ask fest what festivals are in "+campRoot)
	}

	dir, readable := firstFestivalPath(out)
	if !readable {
		return "", errpkg.New(CodeFestivalList, "could not read the festival list fest printed for "+campRoot)
	}
	if dir == "" {
		return "", errpkg.New(CodeNoFestival, "no festival in "+campRoot+" yet")
	}
	return dir, nil
}

// festivalListing is the part of fest list --json the hub reads.
//
// fest emits a key only for the statuses that actually hold festivals, out of
// active, ready, planning, parked and ritual, alongside a total count and
// sometimes a residents object. Decoding into a map of arrays would therefore
// fail on the total, so this takes the three buckets it wants by name and lets
// encoding/json discard everything else.
type festivalListing struct {
	Active   []festivalEntry `json:"active"`
	Ready    []festivalEntry `json:"ready"`
	Planning []festivalEntry `json:"planning"`
}

type festivalEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// firstFestivalPath picks the festival directory fest next should run in. The
// second return separates the two empty answers: a camp fest listed with no
// festivals in it, and output this hub could not read at all.
func firstFestivalPath(out []byte) (dir string, readable bool) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return "", true
	}
	var listing festivalListing
	if err := json.Unmarshal([]byte(trimmed), &listing); err != nil {
		return "", false
	}
	for _, bucket := range [][]festivalEntry{listing.Active, listing.Ready, listing.Planning} {
		for _, entry := range bucket {
			if p := strings.TrimSpace(entry.Path); p != "" {
				return p, true
			}
		}
	}
	return "", true
}
