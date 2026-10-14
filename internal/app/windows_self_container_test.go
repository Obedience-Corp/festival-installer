//go:build windows && container_fs

package app

import (
	"strings"
	"testing"
)

func TestWindowsManagedSelfUpdateGuard(t *testing.T) {
	if err := refuseRunningWindowsSelfReplacement(SelfExternal); err != nil {
		t.Fatalf("external hub must keep SelfSkipped behavior: %v", err)
	}
	if err := refuseRunningWindowsSelfReplacement(SelfUnknown); err != nil {
		t.Fatalf("unknown hub must keep SelfSkipped behavior: %v", err)
	}
	err := refuseRunningWindowsSelfReplacement(SelfManaged)
	if err == nil || !strings.Contains(err.Error(), "scoop update festival") || !strings.Contains(err.Error(), "outside the managed bin") {
		t.Fatalf("managed hub needs actionable Windows lock guidance: %v", err)
	}
}
