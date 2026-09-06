package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// GettingStartedWorkflowName is the workflow fest scaffolds for a camp with
// nothing to run yet. It is also the directory name under workflow/.
const GettingStartedWorkflowName = "getting-started"

// gettingStartedDirs is where the scaffolded workflow lives, relative to the
// camp root. fest writes WORKFLOW.md into the current directory, so the hub
// gives it a directory of its own rather than dropping the document and its
// runtime state at the top of the user's camp.
var gettingStartedDirs = []string{"workflow", GettingStartedWorkflowName}

// workflowDefinition is the document fest create workflow accepts on --steps.
// The field names are fest's, not ours.
type workflowDefinition struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Steps       []workflowStep `json:"steps"`
}

type workflowStep struct {
	Name       string   `json:"name"`
	Goal       string   `json:"goal"`
	Actions    []string `json:"actions"`
	Output     string   `json:"output"`
	Checkpoint string   `json:"checkpoint"`
}

// checkpointNone leaves a step advanceable without an approval round. The tour
// is teaching the loop, and a checkpoint here would stop fest next on a
// question the user has no context to answer yet.
const checkpointNone = "none"

// GettingStartedWorkflow is the compiled-in content of the scaffolded workflow.
// Each step is a real thing a new user does next in their own camp, not a
// description of the hub, so finishing the workflow leaves them somewhere.
//
// It is compiled in for the same reason the launchpad catalog and the tour are:
// content that can disagree with the binary running it is worse than none.
func GettingStartedWorkflow() workflowDefinition {
	return workflowDefinition{
		Title:       "Getting started",
		Description: "Three things to do next in this camp, one fest next at a time.",
		Steps: []workflowStep{
			{
				Name: "Read the festival guide",
				Goal: "Know what a festival is before you make one.",
				Actions: []string{
					"Open festivals/README.md in this camp",
					"Read what phases, sequences and tasks are, and how they nest",
				},
				Output:     "You can say what a phase is and what a task is.",
				Checkpoint: checkpointNone,
			},
			{
				Name: "Capture your first idea",
				Goal: "Get one real thing out of your head and into the camp.",
				Actions: []string{
					`Run: camp idea add "the first thing you want to build"`,
					"Run: camp idea list to see it in the inbox",
				},
				Output:     "One idea recorded in this camp.",
				Checkpoint: checkpointNone,
			},
			{
				Name: "Plan your first festival",
				Goal: "Turn that idea into a plan an agent can execute.",
				Actions: []string{
					"Run: fest create festival",
					"Hand the new festival to an agent to resolve its REPLACE markers",
					"Run: fest next inside it once the markers are filled",
				},
				Output:     "A festival that passes fest validate, with a first task to do.",
				Checkpoint: checkpointNone,
			},
		},
	}
}

// GettingStartedWorkflowDir is where the workflow will be scaffolded for this
// camp. It does not create anything.
func GettingStartedWorkflowDir(campRoot string) string {
	return filepath.Join(append([]string{campRoot}, gettingStartedDirs...)...)
}

// PrepareGettingStartedWorkflow makes the directory fest will write the
// workflow into and returns it along with the inline steps argument.
//
// It creates a directory and nothing else. The workflow itself is fest's to
// write, through the launchpad, so that the hub never becomes a second way of
// producing festival artifacts.
func PrepareGettingStartedWorkflow(ctx context.Context, campRoot string) (dir, steps string, err error) {
	if cerr := ctx.Err(); cerr != nil {
		return "", "", errpkg.Wrap("E_WORKFLOW_CTX", cerr, "context cancelled before preparing the workflow")
	}
	if campRoot == "" {
		return "", "", errpkg.New(CodeNoCampRoot, "no camp here to put a getting started workflow in")
	}
	dir = GettingStartedWorkflowDir(campRoot)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "", "", errpkg.Wrap("E_WORKFLOW_DIR", mkErr, "create "+dir)
	}
	encoded, jsonErr := json.Marshal(GettingStartedWorkflow())
	if jsonErr != nil {
		return "", "", errpkg.Wrap("E_WORKFLOW_STEPS", jsonErr, "encode the getting started steps")
	}
	return dir, string(encoded), nil
}
