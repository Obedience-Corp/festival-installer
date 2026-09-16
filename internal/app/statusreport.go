package app

import (
	"context"
	"os/exec"
	"sync"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// StatusToolEntry is one tool's resolution and version for the app.
type StatusToolEntry struct {
	// Tool is the bare binary name.
	Tool string `json:"tool"`
	// Path is the absolute path ResolveTool returns, empty when nothing resolves.
	Path string `json:"path,omitempty"`
	// Managed is <bin-dir>/<tool> when that file exists, empty otherwise.
	Managed string `json:"managed,omitempty"`
	// Present is true when Path is non-empty.
	Present bool `json:"present"`
	// Version is what the resolved binary reported, empty when it did not run
	// or its output did not parse as a version.
	Version string `json:"version,omitempty"`
	// Package is the marketplace package that owns this binary.
	Package string `json:"package,omitempty"`
	// ReceiptVersion and ReceiptChannel come from that package's receipt row,
	// empty when no receipt exists.
	ReceiptVersion string `json:"receipt_version,omitempty"`
	ReceiptChannel string `json:"receipt_channel,omitempty"`
	// Origin is how this binary got onto the machine.
	Origin OriginKind `json:"origin"`
	// Error is why resolution or the version probe failed, empty on success.
	Error string `json:"error,omitempty"`
}

// StatusPrerequisite is one non-suite binary the installer needs at runtime.
type StatusPrerequisite struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	Path    string `json:"path,omitempty"`
}

// StatusReport is the single answer festival-app reads on launch.
type StatusReport struct {
	Home              string               `json:"home"`
	BinDir            string               `json:"bin_dir"`
	MarketplaceSource string               `json:"marketplace_source"`
	Origin            OriginKind           `json:"origin"`
	Flavor            PackageFlavor        `json:"flavor,omitempty"`
	Setup             SetupState           `json:"setup"`
	Tools             []StatusToolEntry    `json:"tools"`
	Prerequisites     []StatusPrerequisite `json:"prerequisites"`
}

// statusTools is the reporting order. It is fixed because festival-app indexes
// the array positionally in its tests, and because a stable order makes two
// status payloads diffable.
var statusTools = []string{"camp", "fest", selfBinaryName, obeyBinary, obeyDevCLIBinary}

// toolPackage maps a binary to the marketplace package that ships it. It is a
// static map rather than a receipt scan because a tool must report its owning
// package even on a machine where nothing is installed yet.
var toolPackage = map[string]string{
	"camp":           FestivalPackageID,
	"fest":           FestivalPackageID,
	selfBinaryName:   FestivalPackageID,
	obeyBinary:       ObeyPackageID,
	obeyDevCLIBinary: ObeyPackageID,
}

// obeyDevCLIBinary is the developer CLI the obey package ships beside the
// daemon. It is reported but is never an install target of its own.
const obeyDevCLIBinary = "ob"

// versionProbes is how each tool reports its version. The argument sets are
// tried in order and the first that runs and parses wins. Every form they print
// is normalized by ParseToolVersion, so no probe carries a prefix of its own.
var versionProbes = map[string][][]string{
	"camp":         {{"version", "--short"}, {"version"}},
	"fest":         {{"version", "--short"}, {"version"}},
	selfBinaryName: {{"version", "--short"}, {"version"}},
	// obey answers --version until `version --short` is the installed floor,
	// so the subcommand is tried first and the flag is the fallback.
	obeyBinary:       {{"version", "--short"}, {"--version"}},
	obeyDevCLIBinary: {{"--version"}},
}

// prerequisiteBinaries are the non-suite binaries the installer itself needs.
// git is here because the first install clones the marketplace with it
// (internal/source/autoseed.go, internal/source/git.go), and a missing git
// surfaces as E_MARKETPLACE_SEED from deep inside an install rather than as a
// prerequisite the caller could have checked.
var prerequisiteBinaries = []string{"git"}

// StatusReportFor builds the report festival-app reads on launch.
//
// It never creates the installer home and never opens state.db unless it
// already exists, matching every other read-only path in this package. It also
// never grades: a missing tool is a fact for the caller to act on, not a
// failure to report.
func StatusReportFor(ctx context.Context) (StatusReport, error) {
	if err := ctx.Err(); err != nil {
		return StatusReport{}, errpkg.Wrap("E_STATUS_CTX", err, "context cancelled")
	}
	home, err := state.Home(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	cfg, err := state.LoadConfig(ctx, home)
	if err != nil {
		return StatusReport{}, err
	}
	setup, err := ResolveSetupState(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	origin, _ := DetectSuite(ctx)

	return StatusReport{
		Home:              home,
		BinDir:            binDir,
		MarketplaceSource: cfg.Marketplaces.Default,
		Origin:            origin.Kind,
		Flavor:            origin.Flavor,
		Setup:             setup,
		Tools:             resolveStatusTools(ctx),
		Prerequisites:     resolvePrerequisites(),
	}, nil
}

func resolveStatusTools(ctx context.Context) []StatusToolEntry {
	recs := readStatusReceipts(ctx)
	out := make([]StatusToolEntry, len(statusTools))
	var wg sync.WaitGroup
	for i, tool := range statusTools {
		out[i] = StatusToolEntry{
			Tool:    tool,
			Package: toolPackage[tool],
			Managed: ManagedToolPath(ctx, tool),
			Origin:  OriginAbsent,
		}
		if rec, ok := recs[toolPackage[tool]]; ok {
			out[i].ReceiptVersion = rec.Version
			out[i].ReceiptChannel = rec.Channel
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fillToolEntry(ctx, &out[i])
		}(i)
	}
	wg.Wait()
	return out
}

// fillToolEntry resolves one tool and probes its version. Origin is classified
// from the path this tool resolved to rather than from the suite origin,
// because a machine can have a package-manager camp and a managed or leftover
// obey at the same time. The suite origin stays on the report's Origin field.
func fillToolEntry(ctx context.Context, e *StatusToolEntry) {
	path, err := ResolveTool(ctx, e.Tool)
	if err != nil {
		e.Error = err.Error()
		return
	}
	e.Path = path
	e.Present = true
	if e.Managed != "" && samePath(path, e.Managed) {
		e.Origin = OriginManaged
	} else {
		e.Origin, _, _ = classifyPath(ctx, path)
	}
	version, verr := probeToolVersion(ctx, e.Tool, path)
	if verr != nil {
		e.Error = verr.Error()
		return
	}
	e.Version = version
}

// probeToolVersion runs the tool's own version command and returns what it
// reported. A tool that runs and answers something unparseable is reported as
// having no version, with the reason in the error, because a wrong version is
// worse for the app's floor check than an absent one.
func probeToolVersion(ctx context.Context, tool, path string) (string, error) {
	argSets, ok := versionProbes[tool]
	if !ok {
		return "", errpkg.New("E_VERSION_PROBE", "no version probe defined for "+tool)
	}
	runCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	var lastErr error
	for _, args := range argSets {
		out, err := runProbe(runCtx, path, args...)
		if err != nil {
			lastErr = err
			continue
		}
		if v := ParseToolVersion(tool, string(out)); v != "" {
			return v, nil
		}
		lastErr = errpkg.New("E_VERSION_PROBE", tool+" reported an unparseable version: "+firstLine(string(out)))
	}
	if lastErr == nil {
		lastErr = errpkg.New("E_VERSION_PROBE", "no version probe succeeded for "+tool)
	}
	return "", errpkg.Wrap("E_VERSION_PROBE", lastErr, "read "+tool+" version at "+path)
}

// readStatusReceipts loads the receipts for every package a reported tool
// belongs to. A missing database is an empty map, never an error, so a machine
// that has never run the hub still gets a full report.
func readStatusReceipts(ctx context.Context) map[string]receipts.Receipt {
	out := map[string]receipts.Receipt{}
	home, err := state.Home(ctx)
	if err != nil {
		return out
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil || !ok {
		return out
	}
	defer func() { _ = db.Close(ctx) }()
	for _, pkg := range []string{FestivalPackageID, ObeyPackageID} {
		rec, gerr := receipts.Get(ctx, db.Raw(), pkg)
		if gerr == nil {
			out[pkg] = rec
		}
	}
	return out
}

func resolvePrerequisites() []StatusPrerequisite {
	out := make([]StatusPrerequisite, 0, len(prerequisiteBinaries))
	for _, name := range prerequisiteBinaries {
		p := StatusPrerequisite{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			p.Present = true
			p.Path = path
		}
		out = append(out, p)
	}
	return out
}
