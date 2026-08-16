package obey

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const skillManifestFile = "SKILL.md"

type Host struct {
	campaignRoot string
	replace      bool
}

func New(campaignRoot string, replace bool) *Host {
	return &Host{campaignRoot: campaignRoot, replace: replace}
}

func (h *Host) HostID() string { return "obey" }

func ResolveCampaignRoot(ctx context.Context, override string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errpkg.Wrap("E_CAMPAIGN_CTX", err, "context cancelled")
	}
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", errpkg.New("E_CAMPAIGN_ROOT_NOT_ABS", "campaign root must be absolute")
		}
		return override, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", errpkg.Wrap("E_CWD", err, "resolve working directory")
	}
	for dir := wd; ; {
		if fi, err := os.Stat(filepath.Join(dir, ".campaign")); err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errpkg.New("E_CAMPAIGN_ROOT_REQUIRED", "no .campaign found; pass a campaign root")
		}
		dir = parent
	}
}

func (h *Host) skillsRoot() string {
	return filepath.Join(h.campaignRoot, ".campaign", "skills")
}

func (h *Host) TargetPath(entry metadata.InstallEntry) (string, error) {
	if entry.Kind != "skill_bundle" {
		return "", errpkg.New("E_BAD_ENTRY_KIND", "obey activation requires kind skill_bundle")
	}
	if entry.SkillSlug == "" {
		return "", errpkg.New("E_SKILL_SLUG_EMPTY", "install entry missing skill_slug")
	}
	if err := shared.ValidateSegment(entry.SkillSlug); err != nil {
		return "", err
	}
	return filepath.Join(h.skillsRoot(), entry.SkillSlug), nil
}

func (h *Host) Activate(ctx context.Context, staged string, entry metadata.InstallEntry) ([]receipts.OwnedFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, errpkg.Wrap("E_SKILL_CTX", err, "context cancelled")
	}
	if err := validateSkillBundle(staged); err != nil {
		return nil, err
	}
	dst, err := h.TargetPath(entry)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dst); err == nil {
		if !h.replace {
			return nil, errpkg.New("E_SKILL_CONFLICT", "skill slug already exists: "+dst+" (use replace)")
		}
		if err := os.RemoveAll(dst); err != nil {
			return nil, errpkg.Wrap("E_SKILL_REPLACE", err, "removing existing "+dst)
		}
	}
	if err := shared.CopyTreeSafe(staged, dst); err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	records, err := shared.FileRecordsForTree(dst)
	if err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}
	return records, nil
}

func (h *Host) Remove(ctx context.Context, owned []receipts.OwnedFile) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SKILL_CTX", err, "context cancelled")
	}
	return shared.RemoveOwnedAndPrune(owned, h.skillsRoot())
}

func validateSkillBundle(staged string) error {
	data, err := os.ReadFile(filepath.Join(staged, skillManifestFile))
	if err != nil {
		return errpkg.New("E_SKILL_MD_MISSING", "skill bundle missing "+skillManifestFile)
	}
	name, desc, err := parseSkillFrontmatter(string(data))
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errpkg.New("E_SKILL_NAME_EMPTY", "SKILL.md frontmatter name is empty")
	}
	if strings.TrimSpace(desc) == "" {
		return errpkg.New("E_SKILL_DESC_EMPTY", "SKILL.md frontmatter description is empty")
	}
	return nil
}

func parseSkillFrontmatter(content string) (string, string, error) {
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", errpkg.New("E_SKILL_FRONTMATTER", "SKILL.md must begin with --- frontmatter")
	}
	rest := trimmed[len("---"):]
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", errpkg.New("E_SKILL_FRONTMATTER", "SKILL.md frontmatter is not closed with ---")
	}
	block := rest[:end]

	var name, desc string
	for _, line := range strings.Split(block, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}
	return name, desc, nil
}
