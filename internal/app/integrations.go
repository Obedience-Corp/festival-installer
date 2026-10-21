package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/shellsetup"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/lock"
)

const IntegrationSchema = "festival-integrations/v1"

type IntegrationCheck struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	Path       string `json:"path,omitempty"`
	Repairable bool   `json:"repairable"`
}

type IntegrationReport struct {
	Schema      string             `json:"schema"`
	Shell       string             `json:"shell"`
	Ready       bool               `json:"ready"`
	Changed     bool               `json:"changed"`
	NewTerminal bool               `json:"new_terminal"`
	Disabled    bool               `json:"disabled"`
	Checks      []IntegrationCheck `json:"checks"`
}

type IntegrationOptions struct {
	Shell   string
	Check   bool
	Disable bool
	Enable  bool
}

type integrationSettings struct {
	Disabled bool     `json:"disabled"`
	Files    []string `json:"files,omitempty"`
	Helpers  []string `json:"helpers,omitempty"`
}

var integrationMu sync.Mutex

// ReconcileIntegrations is shared by the desktop sidecar and CLI lifecycle.
// Inspection never creates state. Apply takes both an in-process mutex and the
// existing installer lock so concurrent app/CLI repair cannot duplicate blocks.
func ReconcileIntegrations(ctx context.Context, opts IntegrationOptions) (IntegrationReport, error) {
	return reconcileIntegrations(ctx, opts, os.Geteuid())
}

func reconcileIntegrations(ctx context.Context, opts IntegrationOptions, euid int) (IntegrationReport, error) {
	r := IntegrationReport{Schema: IntegrationSchema, Ready: true, Checks: []IntegrationCheck{}}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if opts.Check && (opts.Enable || opts.Disable) || opts.Enable && opts.Disable {
		return r, fmt.Errorf("check, enable and disable options conflict")
	}
	// Never run user startup files or provider hook installers as root, including
	// health checks. sudo -E may retain a user's HOME and otherwise change ownership.
	if euid == 0 {
		r.add("user", "failed", "Terminal integration must run as your normal user, without sudo. Run festival setup integrations as yourself.", "", true)
		return r, nil
	}
	targets, shellErr := integrationShells(ctx, opts.Shell)
	r.Shell = "unknown"
	if len(targets) > 0 {
		r.Shell = targets[0].Name
	}
	home, err := state.Home(ctx)
	if err != nil {
		return r, err
	}
	if !opts.Check {
		integrationMu.Lock()
		defer integrationMu.Unlock()
		l, err := lock.NewFileLock(home)
		if err != nil {
			return r, err
		}
		release, err := l.Acquire(ctx, 15*time.Second)
		if err != nil {
			return r, err
		}
		defer func() { _ = release() }()
	}
	settingsFile, err := shellsetup.Read(filepath.Join(home, "integrations.json"))
	if err != nil {
		return r, err
	}
	var settings integrationSettings
	if settingsFile.Exists {
		if err := json.Unmarshal(settingsFile.Data, &settings); err != nil {
			return r, fmt.Errorf("read integration preferences: %w", err)
		}
	}
	if opts.Enable {
		settings.Disabled = false
	}
	if opts.Disable {
		settings.Disabled = true
	}
	if settings.Disabled {
		r.Disabled, r.Ready = true, false
		for _, target := range targets {
			if !shellsetup.Supported(target.Name) {
				continue
			}
			profiles, err := integrationProfilesFor(ctx, target)
			if err != nil {
				r.add("profile", "failed", err.Error(), target.Path, true)
			}
			for _, path := range profiles {
				settings.Files = addUnique(settings.Files, path)
			}
		}
		for _, path := range settings.Files {
			r.profile(ctx, path, "", home, opts.Check)
		}
		r.Checks = append(r.Checks, IntegrationCheck{ID: "preference", Status: "disabled", Message: "Automatic terminal integration is disabled. Enable it in setup to use ordinary provider commands.", Repairable: true})
	} else if shellErr != nil {
		r.add("shell", "unavailable", shellErr.Error(), "", true)
	} else if !shellsetup.Supported(r.Shell) {
		r.add("shell", "unsupported", "Automatic integration supports Bash, Zsh, Fish and PowerShell. Choose one with --shell, or disable automatic terminal integration in advanced setup.", r.Shell, false)
	} else {
		for _, target := range targets {
			reconcileShell(ctx, &r, &settings, home, opts.Check, target)
		}
	}
	reconcileBridge(ctx, &r, opts.Check)
	if !opts.Check {
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return r, err
		}
		changed, err := settingsFile.Write(ctx, append(data, '\n'), filepath.Join(home, "backups", "integration"))
		if err != nil {
			return r, err
		}
		r.Changed = r.Changed || changed
	}
	r.NewTerminal = r.Changed && !r.Disabled
	return r, ctx.Err()
}

// integrationAfterInstall retains a partial report when binary placement
// succeeded but user configuration could not be completed.
func integrationAfterInstall(ctx context.Context, disabled bool) *IntegrationReport {
	r, err := ReconcileIntegrations(ctx, IntegrationOptions{Disable: disabled})
	if err != nil {
		r.add("setup", "failed", err.Error(), "", true)
	}
	return &r
}

func IntegrationNote(r *IntegrationReport) string {
	if r == nil {
		return ""
	}
	if r.Disabled {
		return "Automatic terminal setup is disabled. Enable it with festival setup integrations --enable."
	}
	if r.Ready {
		if r.NewTerminal {
			return "Terminal integration is configured. Open a new terminal to use it."
		}
		return "Terminal integration is ready."
	}
	var problems []string
	for _, c := range r.Checks {
		if c.Status != "ready" && c.Status != "not_required" {
			problems = append(problems, c.Message)
		}
	}
	return "Components are installed, but setup needs attention: " + strings.Join(problems, " ") + " Retry with festival setup integrations."
}

func (r *IntegrationReport) add(id, status, message, path string, repairable bool) {
	r.Checks = append(r.Checks, IntegrationCheck{ID: id, Status: status, Message: message, Path: path, Repairable: repairable})
	if status != "ready" && status != "not_required" {
		r.Ready = false
	}
}

func addUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func reconcileShell(ctx context.Context, r *IntegrationReport, settings *integrationSettings, home string, check bool, target integrationShellTarget) {
	shell := target.Name
	origin, _ := DetectSuite(ctx)
	var paths, snippets, helpers []string
	wrap := runtime.GOOS != "windows" && shell != "powershell" && shell != "pwsh"
	for _, tool := range []string{"camp", "fest", "obey"} {
		path, _, err := resolveToolFrom(ctx, tool, origin)
		if err != nil && tool == "obey" {
			r.add("wrapper", "not_required", "Install Obey to enable provider control.", "", false)
			wrap = false
			continue
		}
		if err != nil {
			r.add(tool, "not_required", tool+" is not installed.", "", false)
			continue
		}
		if tool == "camp" {
			helpers = append(helpers, "cgo")
		}
		if tool == "fest" {
			helpers = append(helpers, "fgo")
		}
		paths = append(paths, path)
		if tool == "obey" && !wrap {
			r.add("wrapper", "not_required", "PTY provider wrapping is supported in Bash, Zsh and Fish on Linux and macOS.", path, false)
			continue
		}
		// pwsh selects an executable/profile, while both editions use the
		// PowerShell dialect expected by camp/fest's generators.
		dialect := shell
		if dialect == "pwsh" {
			dialect = "powershell"
		}
		args := []string{"shell-init", dialect}
		if tool == "obey" {
			args = []string{"wrap", "shell-init", shell}
		}
		out, err := shellsetup.Run(ctx, path, args...)
		if err != nil || strings.TrimSpace(out) == "" {
			r.add(tool, "upgrade_required", fmt.Sprintf("Update or repair %s: shell integration could not be generated (%v).", tool, err), path, true)
			return
		}
		snippets = append(snippets, out)
	}
	if len(snippets) == 0 {
		for _, profile := range settings.Files {
			r.profile(ctx, profile, "", home, check)
		}
		return
	}
	helper := filepath.Join(home, "shell", "festival."+shell)
	if shell == "pwsh" || shell == "powershell" {
		helper += ".ps1"
	}
	content := shellsetup.Header(shell, paths) + strings.Join(snippets, "\n")
	file, err := shellsetup.Read(helper)
	if err != nil {
		r.add("helper", "failed", err.Error(), helper, true)
		return
	}
	if string(file.Data) != content {
		if check {
			r.add("helper", "stale", "Terminal helpers need configuration or an update.", helper, true)
		} else {
			changed, err := file.Write(ctx, []byte(content), "")
			if err != nil {
				r.add("helper", "failed", err.Error(), helper, true)
				return
			}
			r.Changed = r.Changed || changed
			r.add("helper", "ready", "Terminal helpers are configured.", helper, false)
		}
	} else {
		r.add("helper", "ready", "Terminal helpers are current.", helper, false)
	}
	settings.Helpers = addUnique(settings.Helpers, helper)
	profiles, err := integrationProfilesFor(ctx, target)
	if err != nil {
		r.add("profile", "failed", err.Error(), "", true)
		return
	}
	for _, profile := range profiles {
		settings.Files = addUnique(settings.Files, profile)
		r.profile(ctx, profile, shellsetup.Block(shellsetup.Source(shell, helper)), home, check)
	}
	if !r.Ready {
		return
	}
	args := []string{"-lic", shellsetup.ProbeHelpers(shell, helpers, wrap)}
	if shell == "pwsh" || shell == "powershell" {
		args = []string{"-NoLogo", "-NoExit"}
	}
	var out string
	if shell == "pwsh" || shell == "powershell" {
		out, err = shellsetup.RunProbeInput(ctx, target.Path, shellsetup.ProbeHelpers(shell, helpers, false)+"exit\n", args...)
	} else {
		out, err = shellsetup.RunProbe(ctx, target.Path, args...)
	}
	if err != nil || !strings.Contains(out, "FESTIVAL_INTEGRATION_READY") {
		r.add("fresh_shell", "failed", "The shell did not load the expected helpers. Check custom provider aliases/functions or startup errors, then repair setup.", target.Path, true)
		return
	}
	r.add("fresh_shell", "ready", "New terminals load camp, fest and supported provider helpers.", target.Path, false)
}

func (r *IntegrationReport) profile(ctx context.Context, path, block, home string, check bool) {
	f, err := shellsetup.Read(path)
	if err != nil {
		r.add("profile", "failed", err.Error(), path, true)
		return
	}
	content, err := shellsetup.Merge(string(f.Data), block)
	if err != nil {
		r.add("profile", "failed", err.Error(), path, false)
		return
	}
	if content == string(f.Data) {
		r.add("profile", "ready", "Terminal startup is configured.", path, false)
		return
	}
	if check {
		r.add("profile", "stale", "Terminal startup needs configuration.", path, true)
		return
	}
	changed, err := f.Write(ctx, []byte(content), filepath.Join(home, "backups", "integration"))
	if err != nil {
		r.add("profile", "failed", err.Error(), path, true)
		return
	}
	r.Changed = r.Changed || changed
	r.add("profile", "ready", "Terminal startup is configured. Open a new terminal to use it.", path, false)
}

// A GUI launch may not inherit ZDOTDIR exported by a terminal. Ask Zsh after
// its own .zshenv has run, without loading interactive profiles.
func integrationProfilesFor(ctx context.Context, target integrationShellTarget) ([]string, error) {
	shell := target.Name
	if shell == "pwsh" || shell == "powershell" {
		out, err := shellsetup.Run(ctx, target.Path, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[Console]::Write($PROFILE.CurrentUserAllHosts)")
		if err != nil {
			return nil, err
		}
		path := strings.TrimSpace(out)
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n") {
			return nil, fmt.Errorf("%s did not report an absolute profile path", target.Path)
		}
		return []string{path}, nil
	}
	if shell == "zsh" {
		out, err := shellsetup.Run(ctx, target.Path, "-c", `printf '\nFESTIVAL_ZDOTDIR=%s\n' "${ZDOTDIR:-$HOME}"`)
		if err != nil {
			return nil, err
		}
		const marker = "\nFESTIVAL_ZDOTDIR="
		at := strings.LastIndex(out, marker)
		if at < 0 {
			return nil, fmt.Errorf("zsh did not report its startup directory")
		}
		dir := strings.TrimSuffix(out[at+len(marker):], "\n")
		if !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\r\n") {
			return nil, fmt.Errorf("ZDOTDIR must be an absolute path")
		}
		return []string{filepath.Join(dir, ".zshrc")}, nil
	}
	return integrationProfiles(shell)
}

func integrationProfiles(shell string) ([]string, error) {
	file, err := ShellRCFile(shell)
	if err != nil {
		return nil, err
	}
	if shell != "bash" {
		return []string{file}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	// Bash reads the first existing login file. Source the same managed helper
	// there as well as in .bashrc so Terminal.app and non-login terminals work.
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		path := filepath.Join(home, name)
		if _, err := os.Stat(path); err == nil {
			return []string{file, path}, nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return []string{file, filepath.Join(home, ".bash_profile")}, nil
}

type integrationBridgeStatus struct {
	Provider      string `json:"provider"`
	Installed     bool   `json:"installed"`
	InstallStatus string `json:"install_status"`
}

func reconcileBridge(ctx context.Context, r *IntegrationReport, check bool) {
	path, err := ResolveTool(ctx, "obey")
	if err != nil {
		return
	}
	read := func() ([]integrationBridgeStatus, error) {
		out, err := shellsetup.Run(ctx, path, "bridge", "status", "--json")
		if err != nil {
			return nil, err
		}
		var status []integrationBridgeStatus
		err = json.Unmarshal([]byte(out), &status)
		return status, err
	}
	status, err := read()
	current := func() bool {
		if err != nil || len(status) == 0 {
			return false
		}
		found := map[string]bool{}
		for _, item := range status {
			switch item.Provider {
			case "claude-code", "codex", "grok":
				found[item.Provider] = item.Installed && item.InstallStatus == "current"
			}
		}
		return found["claude-code"] && found["codex"] && found["grok"]
	}
	if !current() && !check {
		if _, err := shellsetup.Run(ctx, path, "bridge", "install", "all", "--scope", "user"); err != nil {
			r.add("bridge", "failed", "Provider hook setup failed: "+err.Error(), path, true)
			return
		}
		r.Changed = true
		status, err = read()
	}
	if !current() {
		r.add("bridge", "stale", "Provider hooks need setup or repair.", path, true)
		return
	}
	r.add("bridge", "ready", "Claude Code, Codex and Grok have current Obey hooks.", path, false)
}
