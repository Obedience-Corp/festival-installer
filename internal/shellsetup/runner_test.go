//go:build unix

package shellsetup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunCancellationKillsBackgroundChildren(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "late-write")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "/bin/sh", "-c", "(sleep 0.3; echo survived > \"$1\") & wait", "sh", marker)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cancelled child kept running")
	}
}
