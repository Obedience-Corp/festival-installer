package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// festProbeTimeout bounds a single fest call. The tour runs these whenever step
// 4 is chosen, so a fest that hangs must not hang the hub.
const festProbeTimeout = 5 * time.Second

// runTargetBudget bounds the whole search. Every candidate costs one fest call
// to find out whether fest will actually run there, and a camp with a long
// backlog of festivals must not turn one keypress into a minute of subprocesses.
const runTargetBudget = 12 * time.Second

// maxRunTargetProbes caps how many candidates are asked about, for the same
// reason. Beyond a handful the answer the user wants is almost certainly not
// further down the list.
const maxRunTargetProbes = 5

// Error codes ResolveRunTarget reports. Callers branch on these rather than on
// message text, because what the hub offers to do next differs for each.
const (
	// CodeNoCampRoot means there is no camp to look inside. Scaffolding a
	// workflow is not the fix for this; creating a camp is.
	CodeNoCampRoot = "E_FESTIVAL_NO_CAMP"
	// CodeNoFestival means the camp is real and holds nothing fest next can run.
	// This is the signal to scaffold a getting-started workflow.
	CodeNoFestival = "E_FESTIVAL_NONE"
	// CodeFestivalList means fest was asked and its answer could not be used.
	// Not the same as an empty camp, and deliberately not treated as one.
	CodeFestivalList = "E_FESTIVAL_LIST"
	// CodeRunTargetUnchecked means the search ran out of probes or time before
	// it could rule every candidate out. The camp has festivals; whether any of
	// them runs is simply not known. Scaffolding here would add a getting
	// started workflow to a camp that may already have real work in it, so the
	// caller reports instead of acting.
	CodeRunTargetUnchecked = "E_FESTIVAL_UNCHECKED"
)

// TargetKind says what sort of thing fest next will be run in.
type TargetKind string

const (
	// TargetFestival is a festival directory from fest's own listing.
	TargetFestival TargetKind = "festival"
	// TargetWorkflow is a standalone WORKFLOW.md directory with its runtime.
	TargetWorkflow TargetKind = "workflow"
)

// RunTarget is a directory step 4 should run fest next in.
type RunTarget struct {
	Dir  string
	Kind TargetKind
	// Ready is true when fest next runs here as it stands. It is false only for
	// a standalone workflow whose last run finished: fest next refuses without
	// an active run and says to start one, so the caller starts a new run first.
	Ready bool
}

// ResolveRunTarget finds where step 4 should run fest next.
//
// Order. A festival from fest's own listing, taking active before ready before
// planning; then a standalone workflow already in the camp; then nothing, which
// is the caller's signal to scaffold a getting started workflow. parked and
// ritual festivals are skipped: one was set aside on purpose, the other is
// recurring machinery, and neither is what a new user came here to start.
//
// The probe rule. Every candidate is confirmed by asking fest itself rather
// than by re-deriving its rules here. A freshly scaffolded festival is not
// runnable until an agent resolves its template markers, and whether that
// blocks depends on the phase type, so a copy of that rule in the hub would
// drift the first time fest changed it. Note that fest validate cannot answer
// this: it reports a marker-laden festival as valid with a zero exit. So each
// candidate is probed with a read-only fest next, which is the same gate the
// real run will hit, and the first candidate fest accepts wins. Probing stops
// at that first acceptance, so a healthy active festival costs one extra call.
//
// Repeat runs. A standalone workflow whose last run finished is still the right
// place to go, but fest next refuses there because no run is active. That
// target comes back with Ready false, and the caller starts a new run before
// running next, which is what fest's own hint says to do. Scaffolding again
// would fail: fest will not overwrite an existing WORKFLOW.md.
//
// Bounds. At most maxRunTargetProbes candidates and runTargetBudget in total,
// so one keypress cannot turn a camp with a long backlog into a minute of
// subprocesses. When those bounds are hit before every candidate has been ruled
// out, the answer is CodeRunTargetUnchecked rather than CodeNoFestival: the
// camp has festivals and the hub simply does not know whether one runs, which
// is not a licence to scaffold over it.
func ResolveRunTarget(ctx context.Context, campRoot string) (RunTarget, error) {
	if err := ctx.Err(); err != nil {
		return RunTarget{}, errpkg.Wrap("E_FESTIVAL_CTX", err, "context cancelled before resolving a run target")
	}
	campRoot = strings.TrimSpace(campRoot)
	if campRoot == "" {
		return RunTarget{}, errpkg.New(CodeNoCampRoot, "no camp here to look for a festival in")
	}
	festPath, err := ResolveTool(ctx, "fest")
	if err != nil {
		return RunTarget{}, err
	}

	budget, cancel := context.WithTimeout(ctx, runTargetBudget)
	defer cancel()

	candidates, err := festivalCandidates(budget, festPath, campRoot)
	if err != nil {
		return RunTarget{}, err
	}
	dir, ok, checkedAll := firstRunnable(budget, festPath, candidates)
	if ok {
		return RunTarget{Dir: dir, Kind: TargetFestival, Ready: true}, nil
	}
	if !checkedAll {
		return RunTarget{}, uncheckedErr(len(candidates), campRoot)
	}
	workflows := workflowCandidates(campRoot)
	wdir, wok, wCheckedAll := firstRunnable(budget, festPath, workflows)
	if wok {
		return RunTarget{Dir: wdir, Kind: TargetWorkflow, Ready: true}, nil
	}
	if !wCheckedAll {
		return RunTarget{}, uncheckedErr(len(candidates), campRoot)
	}
	if len(workflows) > 0 {
		// A workflow that exists but has no active run, which is what a finished
		// getting started loop looks like. Scaffolding again is not the answer:
		// fest refuses to overwrite an existing WORKFLOW.md. Starting a fresh run
		// is, and it is what fest's own hint says to do.
		return RunTarget{Dir: workflows[0], Kind: TargetWorkflow}, nil
	}
	return RunTarget{}, errpkg.New(CodeNoFestival, "nothing in "+campRoot+" that fest next can run yet")
}

// uncheckedErr is the answer when the search stopped early. It names the size
// of the backlog it could not get through, because "the hub gave up after five"
// and "your camp has no festivals" call for completely different reactions.
func uncheckedErr(candidates int, campRoot string) error {
	return errpkg.New(CodeRunTargetUnchecked,
		"gave up checking "+strconv.Itoa(candidates)+" festivals in "+campRoot+
			" before finding one fest next can run")
}

// firstRunnable returns the first directory fest will actually run in. Probing
// stops at the first acceptance, so the common case of a healthy active
// festival costs exactly one fest call.
//
// checkedAll reports whether every candidate was actually ruled out. It is
// false when the probe cap or the time budget stopped the search early, which
// is not the same answer as "none of these work" and must not be treated as one.
func firstRunnable(ctx context.Context, festPath string, dirs []string) (dir string, found, checkedAll bool) {
	for i, candidate := range dirs {
		if i >= maxRunTargetProbes || ctx.Err() != nil {
			return "", false, false
		}
		if festNextWouldRun(ctx, festPath, candidate) {
			return candidate, true, true
		}
	}
	return "", false, true
}

// festNextWouldRun asks fest whether next has something to do in dir. It is a
// read-only question: fest next reports the next task and never advances one,
// so asking costs nothing but the process.
func festNextWouldRun(ctx context.Context, festPath, dir string) bool {
	runCtx, cancel := context.WithTimeout(ctx, festProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, festPath, "next", "--json") //nolint:gosec // path from ResolveTool, args fixed
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// festivalCandidates asks fest which festivals the camp holds, in the order the
// hub prefers: something already running, then something prepared, then
// something still being planned.
func festivalCandidates(ctx context.Context, festPath, campRoot string) ([]string, error) {
	runCtx, cancel := context.WithTimeout(ctx, festProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, festPath, "list", "--json") //nolint:gosec // path from ResolveTool, args fixed
	cmd.Dir = campRoot
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, errpkg.Wrap(CodeFestivalList, err, "ask fest what festivals are in "+campRoot)
	}
	dirs, readable := festivalPaths(out)
	if !readable {
		return nil, errpkg.New(CodeFestivalList, "could not read the festival list fest printed for "+campRoot)
	}
	return dirs, nil
}

// festivalListing is the part of fest list --json the hub reads.
//
// fest emits a key only for the statuses that actually hold festivals, out of
// active, ready, planning, parked and ritual, alongside a total count and
// sometimes a residents object. Decoding into a map of arrays would therefore
// fail on the total, so this takes the three buckets it wants by name and lets
// encoding/json discard everything else.
//
// parked and ritual are left out on purpose. A parked festival was set aside,
// and a ritual is recurring machinery rather than the piece of work a new user
// is here to start.
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

// festivalPaths flattens the listing into preference order. The second return
// separates the two empty answers: a camp fest listed with no festivals in it,
// and output this hub could not read at all.
func festivalPaths(out []byte) (dirs []string, readable bool) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil, true
	}
	var listing festivalListing
	if err := json.Unmarshal([]byte(trimmed), &listing); err != nil {
		return nil, false
	}
	for _, bucket := range [][]festivalEntry{listing.Active, listing.Ready, listing.Planning} {
		for _, entry := range bucket {
			if p := strings.TrimSpace(entry.Path); p != "" {
				dirs = append(dirs, p)
			}
		}
	}
	return dirs, true
}

// workflowCandidates finds standalone workflows already in the camp. fest has
// no camp-wide listing for these the way it does for festivals, so this looks
// for the two things that make one: a WORKFLOW.md next to a .workflow runtime
// directory.
//
// The search is two directories deep from the camp root, which reaches the
// conventional workflow/<name>/ layout without walking a camp full of project
// checkouts. Hidden directories are skipped.
func workflowCandidates(campRoot string) []string {
	var found []string
	if isWorkflowDir(campRoot) {
		found = append(found, campRoot)
	}
	for _, first := range childDirs(campRoot) {
		if isWorkflowDir(first) {
			found = append(found, first)
		}
		for _, second := range childDirs(first) {
			if isWorkflowDir(second) {
				found = append(found, second)
			}
		}
	}
	return found
}

func childDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, filepath.Join(dir, e.Name()))
	}
	return dirs
}

// isWorkflowDir reports whether dir is a standalone workflow: the document and
// the runtime state fest creates beside it. The document alone is not enough,
// because a WORKFLOW.md copied into a repo for reference has no run to continue.
func isWorkflowDir(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "WORKFLOW.md")); err != nil || fi.IsDir() {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, ".workflow"))
	return err == nil && fi.IsDir()
}
