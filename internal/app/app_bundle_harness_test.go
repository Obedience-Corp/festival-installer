//go:build container_fs || app_native

package app

import (
	"context"
	"database/sql"
	"sync"
)

// fakeAppStore is an in-memory appReceiptStore shared by every Read and Write.
type fakeAppStore struct {
	mu        sync.Mutex
	r         *appReceipt
	failWrite error
}

func (f *fakeAppStore) Read(context.Context) (*appReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.r == nil {
		return nil, nil
	}
	c := *f.r
	return &c, nil
}

func (f *fakeAppStore) Write(_ context.Context, r appReceipt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWrite != nil {
		return f.failWrite
	}
	f.r = &r
	return nil
}

func (f *fakeAppStore) factory() func(*sql.DB) appReceiptStore {
	return func(*sql.DB) appReceiptStore { return f }
}
