package app

import (
	"context"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// TourStepKey identifies one tour step in hub_state. Keys are stable strings,
// never positions, so reordering or inserting a step later cannot mark a
// different step complete.
type TourStepKey string

const (
	TourStepInstall  TourStepKey = "install-suite"
	TourStepPath     TourStepKey = "path"
	TourStepCampInit TourStepKey = "camp-init"
	TourStepFestNext TourStepKey = "fest-next"
)

// TourStepState is how far along one step is.
type TourStepState string

const (
	// TourStepTodo is a step nothing has been done about yet.
	TourStepTodo TourStepState = "todo"
	// TourStepPending is a step whose work is done but whose effect has not
	// reached this process yet, which today means a PATH line written to an rc
	// file that the running shell has not read.
	TourStepPending TourStepState = "pending"
	// TourStepDone is a step the hub can see is finished.
	TourStepDone TourStepState = "done"
	// TourStepSkipped is a step the user chose to pass over. It is never
	// reported as done.
	TourStepSkipped TourStepState = "skipped"
)

// tourRecordDone is the value stored for a step whose completion is recorded
// rather than observed.
const tourRecordDone = "done"

// tourRecordSkipped is the value stored for a step the user skipped.
const tourRecordSkipped = "skipped"

// TourStep is one entry in the getting-started tour. Content is compiled in and
// curated, mirroring the launchpad catalog: a tour that can disagree with the
// binary running it is worse than no tour.
type TourStep struct {
	Key    TourStepKey   `json:"key"`
	Title  string        `json:"title"`
	Detail string        `json:"detail"`
	State  TourStepState `json:"state"`
	// Note is extra context for this step's current state, such as the rc file
	// a PATH line was written to. Empty when there is nothing to add.
	Note string `json:"note,omitempty"`
}

// Done reports whether the step is finished. Skipped is deliberately not done.
func (s TourStep) Done() bool { return s.State == TourStepDone }

// Tour is the getting-started tour's rendered state.
type Tour struct {
	Steps     []TourStep `json:"steps"`
	Dismissed bool       `json:"dismissed"`
}

// TourSteps is the compiled-in step list with nothing observed yet. It is the
// single definition of what the tour asks of a new user.
func TourSteps() []TourStep {
	return []TourStep{
		{
			Key:    TourStepInstall,
			Title:  "Install the camp and fest suite",
			Detail: "the hub installs camp and fest into its managed bin dir",
			State:  TourStepTodo,
		},
		{
			Key:    TourStepPath,
			Title:  "Put the managed bin dir on PATH",
			Detail: "so camp and fest run from any shell, not just this one",
			State:  TourStepTodo,
		},
		{
			Key:    TourStepCampInit,
			Title:  "Create your first camp",
			Detail: "camp init makes a workspace and registers it with camp",
			State:  TourStepTodo,
		},
		{
			Key:    TourStepFestNext,
			Title:  "Run fest next in that camp",
			Detail: "done when fest next is launched from here and exits cleanly, which needs a festival in the camp",
			State:  TourStepTodo,
		},
	}
}

// NextStep is the index of the first step still to do, or len(Steps) when none
// is. A skipped step is not a next step: the user already answered it.
func (t Tour) NextStep() int {
	for i, st := range t.Steps {
		if st.State == TourStepTodo {
			return i
		}
	}
	return len(t.Steps)
}

// Complete reports whether every step is done. A skipped step means the tour is
// not complete, which is the honest answer.
func (t Tour) Complete() bool {
	if len(t.Steps) == 0 {
		return false
	}
	for _, st := range t.Steps {
		if !st.Done() {
			return false
		}
	}
	return true
}

// Step returns the step with key, if the tour carries it.
func (t Tour) Step(key TourStepKey) (TourStep, bool) {
	for _, st := range t.Steps {
		if st.Key == key {
			return st, true
		}
	}
	return TourStep{}, false
}

// tourSignals is everything LoadTour observes about the machine, gathered once
// so the tour and the home setup card cannot disagree about the same facts.
type tourSignals struct {
	setup  SetupState
	camps  CampInventory
	rcFile string
	rcDone bool
}

// LoadTour reads the tour's state. Steps 1 through 3 are observed from real
// state: receipts, PATH, and camp's own list of camps. Step 4 has no durable
// artifact to observe, so it is the one step read from what the hub recorded
// when it ran the child.
//
// Progress lives in FESTIVAL_HOME, not in a camp, because a camp is not
// guaranteed to exist when the tour starts. Nothing here creates the home or
// the database.
func LoadTour(ctx context.Context) (Tour, error) {
	if err := ctx.Err(); err != nil {
		return Tour{}, errpkg.Wrap("E_TOUR_CTX", err, "context cancelled before loading the tour")
	}
	setup, err := ResolveSetupState(ctx)
	if err != nil {
		return Tour{}, err
	}
	sig := tourSignals{setup: setup, camps: ProbeCamps(ctx)}
	sig.rcFile, sig.rcDone = shellRCAlreadyWritten(ctx)

	t := Tour{Steps: TourSteps()}
	keys := make([]string, 0, len(t.Steps)+1)
	for _, st := range t.Steps {
		keys = append(keys, state.HubStateTourStepKey(string(st.Key)))
	}
	keys = append(keys, state.HubStateTourDismissed)

	// One open for every key. Opening the database runs the migration ledger
	// check, so a read per key would take SQLite's write lock five times over
	// to read five rows.
	stored, err := HubStateValues(ctx, keys...)
	if err != nil {
		return Tour{}, err
	}
	for i := range t.Steps {
		applyTourStepState(&t.Steps[i], sig, stored[state.HubStateTourStepKey(string(t.Steps[i].Key))])
	}
	t.Dismissed = stored[state.HubStateTourDismissed] == tourRecordDone
	return t, nil
}

// applyTourStepState resolves one step. Only an observed Done is
// unconditionally authoritative: a step the user skipped and then finished
// anyway reads as done, while a skip still counts against an observation that
// is not itself terminal, so pressing s on a step whose PATH line is written
// but not yet active does what the user asked instead of nothing.
func applyTourStepState(step *TourStep, sig tourSignals, recorded string) {
	observed, note, seen := observeTourStep(step.Key, sig)
	if seen && observed == TourStepDone {
		step.State = TourStepDone
		step.Note = ""
		return
	}
	switch recorded {
	case tourRecordDone:
		step.State = TourStepDone
		return
	case tourRecordSkipped:
		step.State = TourStepSkipped
		return
	}
	if seen {
		step.State = observed
		step.Note = note
		return
	}
	step.State = TourStepTodo
	if step.Key == TourStepCampInit && !sig.camps.Installed {
		step.Note = "camp is not installed yet, finish step 1 first"
	}
}

// observeTourStep reports what the machine says about a step, and whether it
// says anything at all. Only a state the hub can see is returned here; steps
// with no observable signal fall through to what was recorded.
func observeTourStep(key TourStepKey, sig tourSignals) (TourStepState, string, bool) {
	switch key {
	case TourStepInstall:
		if sig.setup.HasReceipts {
			return TourStepDone, "", true
		}
	case TourStepPath:
		if sig.setup.ManagedBinOnPath {
			return TourStepDone, "", true
		}
		// The rc line is written but this process's PATH predates it. That is
		// correct behavior which reads as failure, so it gets its own state
		// instead of looking like nothing happened.
		if sig.rcDone {
			return TourStepPending, "written to " + sig.rcFile + ", restart your shell", true
		}
	case TourStepCampInit:
		if sig.camps.Any() {
			return TourStepDone, "", true
		}
	case TourStepFestNext:
		// No durable artifact proves fest next ran. See the step's own detail.
	}
	return TourStepTodo, "", false
}

// shellRCAlreadyWritten reports the rc file for the user's shell and whether the
// festival block is already in it. Any failure to plan the append reads as not
// written: the tour must render on a machine whose home directory or shell the
// hub cannot make sense of.
func shellRCAlreadyWritten(ctx context.Context) (string, bool) {
	plan, err := PlanShellRCAppend(ctx, ShellFromEnv())
	if err != nil {
		return "", false
	}
	return plan.File, plan.Present
}

// RecordTourStepDone persists that a step finished. It is for steps whose
// completion cannot be observed later, and it is written the moment the hub
// sees the evidence rather than at the end of the tour.
func RecordTourStepDone(ctx context.Context, key TourStepKey) error {
	return setTourStepRecord(ctx, key, tourRecordDone)
}

// SkipTourStep records that the user passed over a step. A skipped step renders
// as skipped and never as done, and the difference survives a restart.
func SkipTourStep(ctx context.Context, key TourStepKey) error {
	return setTourStepRecord(ctx, key, tourRecordSkipped)
}

// DismissTour records that the user closed the tour deliberately. Leaving the
// tour screen does not do this: only an explicit action does.
func DismissTour(ctx context.Context) error {
	return SetHubState(ctx, state.HubStateTourDismissed, tourRecordDone)
}

func setTourStepRecord(ctx context.Context, key TourStepKey, value string) error {
	if !knownTourStep(key) {
		return errpkg.New("E_TOUR_STEP", "unknown tour step: "+string(key))
	}
	return SetHubState(ctx, state.HubStateTourStepKey(string(key)), value)
}

func knownTourStep(key TourStepKey) bool {
	for _, st := range TourSteps() {
		if st.Key == key {
			return true
		}
	}
	return false
}
