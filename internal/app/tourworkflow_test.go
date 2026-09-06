package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestPrepareGettingStartedWorkflow_MakesTheDirectoryAndNothingElse(t *testing.T) {
	camp := t.TempDir()

	dir, steps, err := PrepareGettingStartedWorkflow(context.Background(), camp)
	if err != nil {
		t.Fatalf("PrepareGettingStartedWorkflow: %v", err)
	}
	if want := filepath.Join(camp, "workflow", GettingStartedWorkflowName); dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("the workflow directory must exist: %v", err)
	}
	// fest writes the document and its runtime. The hub makes the directory and
	// stops, so it never becomes a second way of producing festival artifacts.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("the hub must not write anything into the directory, found %d entries", len(entries))
	}
	if steps == "" {
		t.Fatal("the steps argument must not be empty")
	}
}

// TestGettingStartedWorkflow_MatchesFestsShape pins the document against the
// fields fest create workflow reads. A step fest cannot parse is a workflow the
// user never sees.
func TestGettingStartedWorkflow_MatchesFestsShape(t *testing.T) {
	_, steps, err := PrepareGettingStartedWorkflow(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("PrepareGettingStartedWorkflow: %v", err)
	}
	var doc struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Steps       []struct {
			Name       string   `json:"name"`
			Goal       string   `json:"goal"`
			Actions    []string `json:"actions"`
			Output     string   `json:"output"`
			Checkpoint string   `json:"checkpoint"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(steps), &doc); err != nil {
		t.Fatalf("fest could not have parsed this: %v", err)
	}
	if doc.Title == "" || doc.Description == "" {
		t.Fatal("the workflow needs a title and a description")
	}
	if len(doc.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(doc.Steps))
	}
	for i, st := range doc.Steps {
		if st.Name == "" || st.Goal == "" || st.Output == "" {
			t.Fatalf("step %d is missing name, goal or output: %+v", i+1, st)
		}
		if len(st.Actions) == 0 {
			t.Fatalf("step %d has no actions, so there is nothing to do", i+1)
		}
		// A checkpoint would stop fest next on an approval the user has no
		// context to give while they are still learning the loop.
		if st.Checkpoint != checkpointNone {
			t.Fatalf("step %d checkpoint = %q, want %q", i+1, st.Checkpoint, checkpointNone)
		}
	}
}

// TestGettingStartedWorkflow_TellsThemToUseRealCommands keeps the content about
// the user's own camp rather than about the hub.
func TestGettingStartedWorkflow_TellsThemToUseRealCommands(t *testing.T) {
	_, steps, err := PrepareGettingStartedWorkflow(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("PrepareGettingStartedWorkflow: %v", err)
	}
	for _, want := range []string{"festivals/README.md", "camp idea add", "fest create festival"} {
		if !strings.Contains(steps, want) {
			t.Fatalf("the workflow must name %q, got:\n%s", want, steps)
		}
	}
}

func TestPrepareGettingStartedWorkflow_NeedsACamp(t *testing.T) {
	_, _, err := PrepareGettingStartedWorkflow(context.Background(), "")
	if got := errpkg.Code(err); got != CodeNoCampRoot {
		t.Fatalf("code = %q, want %q (err=%v)", got, CodeNoCampRoot, err)
	}
}

func TestPrepareGettingStartedWorkflow_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	camp := t.TempDir()
	if _, _, err := PrepareGettingStartedWorkflow(ctx, camp); err == nil {
		t.Fatal("a cancelled context must not create anything")
	}
	if _, err := os.Stat(filepath.Join(camp, "workflow")); !os.IsNotExist(err) {
		t.Fatal("a cancelled call must not have made the directory")
	}
}
