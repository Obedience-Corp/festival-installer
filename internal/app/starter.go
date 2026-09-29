package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StarterResult is Camp's setup result, plus a failed/deferred action when the
// installed Camp cannot complete it. Binary installation remains successful.
type StarterResult struct {
	SchemaVersion int    `json:"schema_version"`
	Action        string `json:"action"`
	Path          string `json:"path,omitempty"`
	Message       string `json:"message,omitempty"`
}

func (r *StarterResult) Notice() string {
	if r == nil {
		return ""
	}
	if r.Action == "created" || r.Action == "registered" {
		notice := fmt.Sprintf("Your festival camp is ready at %s. Enter it with: cd %s", r.Path, "'"+strings.ReplaceAll(r.Path, "'", "'\"'\"'")+"'")
		if r.Message != "" {
			notice += "\n" + r.Message
		}
		return notice
	}
	return r.Message
}

// SetupStarter executes the selected Camp, with its sibling Fest first on PATH.
// It is used only by explicit setup/install/update operations, never status.
func SetupStarter(ctx context.Context, campPath string) *StarterResult {
	if os.Geteuid() == 0 {
		return &StarterResult{Action: "deferred", Message: "Starter camp setup is deferred. Run festival setup as your regular user."}
	}
	if campPath == "" {
		resolved, err := ResolveWhich(ctx, "camp")
		if err != nil {
			return &StarterResult{Action: "failed", Message: "Starter camp setup is pending: " + err.Error() + ". Run festival setup to retry."}
		}
		campPath = resolved.Path
		if resolved.Managed != "" && resolved.Origin != OriginPackage {
			campPath = resolved.Managed
		}
	}
	return executeStarter(ctx, campPath)
}

func executeStarter(ctx context.Context, campPath string) *StarterResult {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, campPath, "setup", "--json")
	cmd.WaitDelay = 2 * time.Second
	// The current shell can be inside another camp; setup always targets the
	// user's configured camps directory, never that shell's active workspace.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PATH=") && !strings.HasPrefix(value, "CAMP_ROOT=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(campPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return &StarterResult{Action: "failed", Message: "Starter camp setup is pending: " + detail + ". Run festival setup to retry (update Camp if setup is unavailable)."}
	}
	return parseStarterResult(output)
}

func parseStarterResult(output []byte) *StarterResult {
	var result StarterResult
	err := json.Unmarshal(output, &result)
	if err != nil || result.SchemaVersion != 1 || !validStarterAction(result.Action) || ((result.Action == "created" || result.Action == "registered") && result.Path == "") {
		return &StarterResult{Action: "failed", Message: "Camp returned an unsupported setup result. Update Camp and run festival setup to retry."}
	}
	return &result
}

func validStarterAction(action string) bool {
	switch action {
	case "created", "registered", "existing", "complete", "skipped":
		return true
	default:
		return false
	}
}
