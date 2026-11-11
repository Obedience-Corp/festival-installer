package app

import (
	"context"

	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

// InstallApp installs or updates Festival App the standard way for this system (D002).
func InstallApp(ctx context.Context, opts AppInstallOptions) (AppResult, error) {
	receipts, err := productionAppReceipts(ctx)
	if err != nil {
		return AppResult{}, err
	}
	ai, err := newAppInstaller(ctx, receipts)
	if err != nil {
		return AppResult{}, err
	}
	return ai.install(ctx, opts)
}

// appUpdaterKey is the trust root for App artifacts. It is a variable only so
// WithUpdaterKeyForTest can swap it; no flag, environment variable or production
// path reaches it (same contract as source.WithPinnedKeyStoreForTest).
var appUpdaterKey = verify.FestivalAppUpdaterKey

// WithUpdaterKeyForTest swaps the App updater key for one test and returns a
// restore function. Only tests call it.
func WithUpdaterKeyForTest(k verify.MinisignPublicKey) (restore func()) {
	prev := appUpdaterKey
	appUpdaterKey = func() verify.MinisignPublicKey { return k }
	return func() { appUpdaterKey = prev }
}
