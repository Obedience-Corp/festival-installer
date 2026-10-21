package shellsetup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeMigratesAndRemovesOnlyOwnedBlock(t *testing.T) {
	before := "# custom\nalias ll='ls -l'\n" + Block("export PATH=/old:$PATH") + "export EDITOR=vim\n"
	want := "# custom\nalias ll='ls -l'\n" + Block("source '/new helper'") + "export EDITOR=vim\n"
	got, err := Merge(before, Block("source '/new helper'"))
	if err != nil || got != want {
		t.Fatalf("migration: %q, %v", got, err)
	}
	again, err := Merge(got, Block("source '/new helper'"))
	if err != nil || again != got {
		t.Fatalf("idempotence: %q, %v", again, err)
	}
	removed, err := Merge(got, "")
	if err != nil || removed != "# custom\nalias ll='ls -l'\nexport EDITOR=vim\n" {
		t.Fatalf("removal: %q, %v", removed, err)
	}
}

func TestMergeRejectsBrokenMarkers(t *testing.T) {
	for _, text := range []string{Begin + "\n", End + "\n", Block("one") + Block("two"), End + "\n" + Begin + "\n"} {
		if _, err := Merge(text, Block("new")); err == nil {
			t.Fatalf("accepted malformed profile %q", text)
		}
	}
	// A mention inside a comment or a command is not an owned marker line.
	text := "echo '" + Begin + "'\n"
	got, err := Merge(text, Block("new"))
	if err != nil || !strings.HasPrefix(got, text) {
		t.Fatalf("lost unrelated text: %q %v", got, err)
	}
}

func TestWritePreservesSymlinkPermissionsBackupAndUnchangedMtime(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "profile"), filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("original\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	f, err := Read(link)
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(dir, "backups")
	if changed, err := f.Write(context.Background(), []byte("changed\n"), backups); err != nil || !changed {
		t.Fatalf("write: %v %v", changed, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("profile link was replaced")
	}
	info, err = os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal("permissions changed")
	}
	entries, err := os.ReadDir(backups)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backup: %v %v", entries, err)
	}
	body, err := os.ReadFile(filepath.Join(backups, entries[0].Name()))
	if err != nil || string(body) != "original\n" {
		t.Fatalf("backup lost: %q %v", body, err)
	}
	f, err = Read(link)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := f.Write(context.Background(), []byte("changed\n"), backups); err != nil || changed {
		t.Fatalf("repeat write: %v %v", changed, err)
	}
	after, _ := os.Stat(target)
	if !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged file rewritten")
	}
}

func TestWriteRefusesChangedSnapshotAndCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile")
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(context.Background(), []byte("installer"), ""); err == nil {
		t.Fatal("overwrote concurrent edit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Write(ctx, []byte("installer"), ""); err == nil {
		t.Fatal("ignored cancellation")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "user edit" {
		t.Fatal("user edit lost")
	}
}

func TestWriteRepairsIncompleteBackupBeforeReplacingProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile")
	original := []byte("user content that must survive\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(dir, "backups")
	if err := os.Mkdir(backups, 0700); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("%x-%x", sha256.Sum256([]byte(f.Target)), sha256.Sum256(original))
	backup := filepath.Join(backups, name)
	if err := os.WriteFile(backup, original[:5], 0600); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.Write(context.Background(), []byte("replacement"), backups); err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	got, err := os.ReadFile(backup)
	if err != nil || string(got) != string(original) {
		t.Fatalf("incomplete backup: %q %v", got, err)
	}
}
