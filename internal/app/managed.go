package app

import "context"

type managedToolchainKey struct{}

// WithManagedToolchain selects a separately installed, verified user toolchain.
// Package-manager files remain untouched. Only installer-owned bin files are
// written, and every status/resolve/setup operation uses the same selection.
func WithManagedToolchain(ctx context.Context) context.Context {
	return context.WithValue(ctx, managedToolchainKey{}, true)
}

func managedToolchain(ctx context.Context) bool {
	enabled, _ := ctx.Value(managedToolchainKey{}).(bool)
	return enabled
}

func installManagedUpdate(ctx context.Context, target string, opts UpdateOptions) (UpdateResult, string, error) {
	res, err := InstallTarget(ctx, target, InstallOptions{
		Channel: opts.ChannelOverride, Verify: opts.Verify, Progress: opts.Progress,
		NoRestart: opts.NoRestart,
	})
	if err != nil {
		return UpdateResult{}, "", err
	}
	return UpdateResult{Package: res.Package, Action: "upgraded", Version: res.Version,
		Service: res.Service, Starter: res.Starter}, "Using a Festival-managed copy; external package files were preserved.", nil
}
