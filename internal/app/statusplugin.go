package app

import (
	"context"
	"encoding/json"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// StatusReportForPlugins adds only explicitly selected plugins to the read-only
// tool report. It neither discovers nor executes unrelated installed plugins.
func StatusReportForPlugins(ctx context.Context, selected []string) (StatusReport, error) {
	targets := make([]PluginTarget, 0, len(selected))
	seen := make(map[string]bool)
	for _, selector := range selected {
		target, err := ParsePluginTarget(selector)
		if err != nil {
			return StatusReport{}, err
		}
		if target.PackageID == "" {
			return StatusReport{}, errpkg.New("E_PLUGIN_TARGET", "status --plugin requires the full marketplace package ID")
		}
		tool := target.Host + "-" + target.Name
		if seen[tool] {
			return StatusReport{}, errpkg.New("E_PLUGIN_TARGET", "duplicate selected plugin executable "+tool)
		}
		seen[tool] = true
		targets = append(targets, target)
	}
	report, err := StatusReportFor(ctx)
	if err != nil {
		return StatusReport{}, err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return StatusReport{}, errpkg.Wrap("E_STATUS_CTX", err, "read selected plugins")
		}
		report.Tools = append(report.Tools, selectedPluginStatus(ctx, target))
	}
	return report, nil
}

func selectedPluginStatus(ctx context.Context, target PluginTarget) StatusToolEntry {
	tool := target.Host + "-" + target.Name
	entry := StatusToolEntry{Tool: tool, Package: target.PackageID, Origin: OriginAbsent,
		Managed: ManagedToolPath(ctx, tool)}
	if rec, ok := readStatusReceipts(ctx, target.PackageID)[target.PackageID]; ok {
		entry.ReceiptVersion, entry.ReceiptChannel = rec.Version, rec.Channel
	}
	path, _, err := resolveToolFrom(ctx, tool, SuiteOrigin{})
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	entry.Path, entry.Present, entry.Origin = path, true, OriginLeftover
	if entry.Managed != "" && samePath(path, entry.Managed) && entry.ReceiptVersion != "" {
		entry.Origin = OriginManaged
	}
	if tool == "camp-app" {
		probeCampApp(ctx, &entry)
		return entry
	}
	out, err := probeOnce(ctx, path, "version")
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	entry.Version = ParseToolVersion(tool, string(out))
	if entry.Version == "" {
		entry.Error = "plugin reported no usable version"
	}
	return entry
}

func probeCampApp(ctx context.Context, entry *StatusToolEntry) {
	out, err := probeOnce(ctx, entry.Path, "version", "--json")
	if err != nil {
		entry.Error = err.Error()
		return
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Version       string   `json:"version"`
			ProtocolMajor int      `json:"protocolMajor"`
			Capabilities  []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || !result.OK || !LooksLikeVersion(result.Data.Version) {
		entry.Error = "camp-app version --json did not report a successful version/capability payload"
		return
	}
	entry.Version = result.Data.Version
	entry.ProtocolMajor = result.Data.ProtocolMajor
	entry.Capabilities = result.Data.Capabilities
}
