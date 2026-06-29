package obey_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/obey-installer/internal/hosts/obey"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
)

func skillEntry() metadata.InstallEntry {
	return metadata.InstallEntry{Kind: "skill_bundle", SkillSlug: "demo"}
}

func validSkillMd() string {
	return "---\nname: demo\ndescription: A demo skill.\n---\n\n# Demo\n"
}

func stagedBundle(t *testing.T, skillMd string) string {
	t.Helper()
	dir := t.TempDir()
	if skillMd != "" {
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMd), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "helper.md"), []byte("helper"), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return dir
}

func campaignDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".campaign"), 0o755); err != nil {
		t.Fatalf("mkdir .campaign: %v", err)
	}
	return dir
}

func TestActivate_ValidBundleInstallsNoProjection(t *testing.T) {
	root := campaignDir(t)
	h := obey.New(root, false)

	records, err := h.Activate(context.Background(), stagedBundle(t, validSkillMd()), skillEntry())
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	skillDir := filepath.Join(root, ".campaign", "skills", "demo")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md not placed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 owned files, got %d", len(records))
	}
	for _, r := range records {
		if r.Hash == "" {
			t.Fatalf("missing sha256 on %s", r.Path)
		}
	}
	_ = filepath.WalkDir(root, func(path string, _ os.DirEntry, _ error) error {
		if strings.Contains(path, filepath.Join(".claude", "skills")) || strings.Contains(path, filepath.Join(".agents", "skills")) {
			t.Fatalf("projection path must not be written: %s", path)
		}
		return nil
	})
}

func TestActivate_MissingSkillMd(t *testing.T) {
	root := campaignDir(t)
	h := obey.New(root, false)
	if _, err := h.Activate(context.Background(), stagedBundle(t, ""), skillEntry()); err == nil || !strings.Contains(err.Error(), "E_SKILL_MD_MISSING") {
		t.Fatalf("expected E_SKILL_MD_MISSING, got %v", err)
	}
}

func TestActivate_BadFrontmatter(t *testing.T) {
	root := campaignDir(t)
	h := obey.New(root, false)
	cases := map[string]string{
		"no fence":   "name: demo\ndescription: x\n",
		"empty name": "---\nname: \ndescription: x\n---\n",
		"empty desc": "---\nname: demo\ndescription: \n---\n",
	}
	for label, md := range cases {
		t.Run(label, func(t *testing.T) {
			_, err := h.Activate(context.Background(), stagedBundle(t, md), skillEntry())
			if err == nil || !strings.Contains(err.Error(), "E_SKILL_") {
				t.Fatalf("expected E_SKILL_* error, got %v", err)
			}
		})
	}
}

func TestActivate_ConflictWithoutReplace(t *testing.T) {
	root := campaignDir(t)
	existing := filepath.Join(root, ".campaign", "skills", "demo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existing, "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("old: %v", err)
	}
	h := obey.New(root, false)
	if _, err := h.Activate(context.Background(), stagedBundle(t, validSkillMd()), skillEntry()); err == nil || !strings.Contains(err.Error(), "E_SKILL_CONFLICT") {
		t.Fatalf("expected E_SKILL_CONFLICT, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(existing, "old.md")); err != nil {
		t.Fatalf("existing skill should be untouched: %v", err)
	}
}

func TestActivate_ConflictWithReplace(t *testing.T) {
	root := campaignDir(t)
	existing := filepath.Join(root, ".campaign", "skills", "demo")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existing, "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("old: %v", err)
	}
	h := obey.New(root, true)
	if _, err := h.Activate(context.Background(), stagedBundle(t, validSkillMd()), skillEntry()); err != nil {
		t.Fatalf("Activate with replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(existing, "old.md")); !os.IsNotExist(err) {
		t.Fatal("old content should be gone after replace")
	}
	if _, err := os.Stat(filepath.Join(existing, "SKILL.md")); err != nil {
		t.Fatalf("new SKILL.md should be present: %v", err)
	}
}

func TestActivate_SymlinkRejected(t *testing.T) {
	root := campaignDir(t)
	staged := t.TempDir()
	if err := os.WriteFile(filepath.Join(staged, "SKILL.md"), []byte(validSkillMd()), 0o644); err != nil {
		t.Fatalf("SKILL.md: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(staged, "evil")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	h := obey.New(root, false)
	if _, err := h.Activate(context.Background(), staged, skillEntry()); err == nil || !strings.Contains(err.Error(), "E_TREE_UNSAFE") {
		t.Fatalf("expected E_TREE_UNSAFE, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".campaign", "skills", "demo")); !os.IsNotExist(statErr) {
		t.Fatal("nothing should remain after a rejected unsafe bundle")
	}
}

func TestActivate_RejectsUnsafeSlug(t *testing.T) {
	root := campaignDir(t)
	h := obey.New(root, false)

	entry := metadata.InstallEntry{Kind: "skill_bundle", SkillSlug: "../escape"}
	_, err := h.Activate(context.Background(), stagedBundle(t, validSkillMd()), entry)
	if err == nil || !strings.Contains(err.Error(), "E_HOST_UNSAFE_NAME") {
		t.Fatalf("expected E_HOST_UNSAFE_NAME, got %v", err)
	}
	escaped := filepath.Join(root, ".campaign", "escape")
	if _, statErr := os.Stat(escaped); statErr == nil {
		t.Fatalf("skill escaped its root to %s", escaped)
	}
}

func TestActivateThenRemove(t *testing.T) {
	root := campaignDir(t)
	h := obey.New(root, false)
	records, err := h.Activate(context.Background(), stagedBundle(t, validSkillMd()), skillEntry())
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	sibling := filepath.Join(root, ".campaign", "skills", "other")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("sibling: %v", err)
	}
	if err := h.Remove(context.Background(), records); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".campaign", "skills", "demo")); !os.IsNotExist(statErr) {
		t.Fatal("demo skill dir should be pruned")
	}
	if _, statErr := os.Stat(sibling); statErr != nil {
		t.Fatalf("sibling skill should remain: %v", statErr)
	}
}

func TestResolveCampaignRoot_Required(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := obey.ResolveCampaignRoot(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "E_CAMPAIGN_ROOT_REQUIRED") {
		t.Fatalf("expected E_CAMPAIGN_ROOT_REQUIRED, got %v", err)
	}
}

func TestResolveCampaignRoot_Detects(t *testing.T) {
	root := campaignDir(t)
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(sub)
	got, err := obey.ResolveCampaignRoot(context.Background(), "")
	if err != nil {
		t.Fatalf("ResolveCampaignRoot: %v", err)
	}
	if resolveSym(got) != resolveSym(root) {
		t.Fatalf("detected %s, want %s", got, root)
	}
}

func resolveSym(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
