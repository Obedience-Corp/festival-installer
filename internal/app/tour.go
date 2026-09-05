package app

import (
	"context"
	"strconv"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// TourStep is one step of the getting-started tour. The content is compiled in
// rather than loaded from a data file, matching the launchpad catalog: a tour
// that can be out of sync with the binary running it is worse than no tour.
type TourStep struct {
	// Title is the one-line instruction shown in the step list.
	Title string `json:"title"`
	// Detail says what the hub will actually do, or what the user must do.
	Detail string `json:"detail"`
	// Done is true once the hub can see this step is finished.
	Done bool `json:"done"`
}

// Tour is the getting-started tour's rendered state.
type Tour struct {
	// Steps are always all four, in order, whatever their state.
	Steps []TourStep `json:"steps"`
	// Dismissed is true once the user closed the tour deliberately.
	Dismissed bool `json:"dismissed"`
}

// TourSteps is the compiled-in step list, with nothing marked done. It is the
// single definition of what the tour asks of a new user.
func TourSteps() []TourStep {
	return []TourStep{
		{
			Title:  "Install the camp and fest suite",
			Detail: "the hub downloads camp and fest into its managed bin dir",
		},
		{
			Title:  "Put the managed bin dir on PATH",
			Detail: "so camp and fest run from any shell, not just this one",
		},
		{
			Title:  "Create your first camp",
			Detail: "camp init sets up a workspace and registers it",
		},
		{
			Title:  "Run fest next in that camp",
			Detail: "the launchpad hands the terminal to fest and takes it back",
		},
	}
}

// NextStep is the index of the first step still to do, or len(Steps) when the
// tour is finished.
func (t Tour) NextStep() int {
	for i, st := range t.Steps {
		if !st.Done {
			return i
		}
	}
	return len(t.Steps)
}

// Complete reports whether every step is done.
func (t Tour) Complete() bool { return t.NextStep() >= len(t.Steps) }

// LoadTour reads the tour's state from FESTIVAL_HOME. Progress is persisted
// rather than held in the process, so quitting the hub and relaunching it
// returns to the same place.
//
// It never creates the home or the database: a machine the hub has never
// written to reports a tour with nothing done, which is the correct answer.
func LoadTour(ctx context.Context) (Tour, error) {
	if err := ctx.Err(); err != nil {
		return Tour{}, errpkg.Wrap("E_TOUR_CTX", err, "context cancelled before loading the tour")
	}
	t := Tour{Steps: TourSteps()}

	reached, err := tourReachedStep(ctx)
	if err != nil {
		return Tour{}, err
	}
	for i := range t.Steps {
		t.Steps[i].Done = i < reached
	}

	dismissed, ok, err := HubState(ctx, state.HubStateTourDismissed)
	if err != nil {
		return Tour{}, err
	}
	t.Dismissed = ok && dismissed == "true"
	return t, nil
}

// RecordTourStep persists that the user has finished step index (zero based),
// never moving the tour backwards: a step observed as done stays done even if
// a later read of the same signal is noisier.
func RecordTourStep(ctx context.Context, index int) error {
	if index < 0 {
		return errpkg.New("E_TOUR_STEP", "tour step index cannot be negative")
	}
	reached, err := tourReachedStep(ctx)
	if err != nil {
		return err
	}
	if index+1 <= reached {
		return nil
	}
	return SetHubState(ctx, state.HubStateTourStep, strconv.Itoa(index+1))
}

// tourReachedStep is the count of steps recorded as done. An unparseable or
// negative stored value is treated as no progress rather than as an error: the
// tour is guidance, and refusing to render it because one row is corrupt would
// strand exactly the user it exists for.
func tourReachedStep(ctx context.Context) (int, error) {
	raw, ok, err := HubState(ctx, state.HubStateTourStep)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	reached, convErr := strconv.Atoi(raw)
	if convErr != nil || reached < 0 {
		return 0, nil
	}
	return reached, nil
}
