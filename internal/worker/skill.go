package worker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skill.go — skills in the Agent Skills directory format, with the one
// Sinet restriction (Spec S08.1): skill files are STATIC — no
// load-time-executing dynamic content, ever. The restriction is enforced
// structurally, not by prose: the frontmatter schema is a closed allowlist
// with no hook/command/script slot, no file in a skill dir may carry an
// executable bit, and symlinks are refused (a symlink could smuggle
// content from outside the store). Skill bodies are screened at station 1
// like template bodies (Spec S08.10 — the P-T14-2 carrier class; measured:
// 91% of a public registry's malicious skills carried injection).

// Skill is one loaded skill directory.
type Skill struct {
	Name        string
	Dir         string
	Description string
	Body        string
	// Files are the auxiliary file paths (relative to Dir), SKILL.md
	// excluded.
	Files []string
	// Findings are static-packaging violations found at load; station 1
	// treats them as errors.
	Findings []Finding
}

// skillFrontmatterAllow is the closed SKILL.md frontmatter schema. There
// is deliberately no key that could name an executable, a hook, or a
// config channel.
var skillFrontmatterAllow = map[string]bool{
	"name":        true,
	"description": true,
	"version":     true,
	"license":     true,
}

// LoadSkill loads and statically validates one skill directory.
func LoadSkill(dir string) (Skill, error) {
	sk := Skill{Dir: dir}
	raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return sk, fmt.Errorf("%w: skill dir %s: %v", ErrInvalid, dir, err)
	}
	fmLines, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return sk, fmt.Errorf("%w: SKILL.md in %s: %v", ErrInvalid, dir, err)
	}
	fm, err := parseFrontmatter(fmLines)
	if err != nil {
		return sk, fmt.Errorf("%w: SKILL.md frontmatter in %s: %v", ErrInvalid, dir, err)
	}
	sk.Body = strings.TrimSpace(body)

	addf := func(code, msg string) { sk.Findings = append(sk.Findings, Finding{Code: code, Message: msg}) }
	for _, path := range fm.keyPaths("") {
		top, _, _ := strings.Cut(path, ".")
		if !skillFrontmatterAllow[top] || strings.Contains(path, ".") {
			classifyUnknownKey("SKILL.md "+path, addf)
		}
	}
	if v, ok := fm.get("name"); ok && v.IsScalar {
		sk.Name = v.Scalar
	}
	if v, ok := fm.get("description"); ok && v.IsScalar {
		sk.Description = v.Scalar
	}
	if sk.Name == "" {
		addf(FindingSchema, "SKILL.md: name required")
	} else if !nameRe.MatchString(sk.Name) {
		addf(FindingConvention, fmt.Sprintf("SKILL.md name %q: must be lowercase dash-separated", sk.Name))
	}
	if sk.Description == "" {
		addf(FindingSchema, "SKILL.md: description required")
	}

	// Static-only walk: every file inspected; exec bits and symlinks are
	// packaging violations (Spec S08.1 static-only; P-T14-2).
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			addf(FindingSchema, fmt.Sprintf("%s: symlinks are not accepted in a skill dir (static-only, S08.1)", rel))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info.Mode()&0o111 != 0 {
			addf(FindingSchema, fmt.Sprintf("%s: executable bit set — skill files are static, never load-time-executing (S08.1)", rel))
		}
		if rel != "SKILL.md" {
			sk.Files = append(sk.Files, rel)
		}
		return nil
	})
	if err != nil {
		return sk, fmt.Errorf("%w: walk skill dir %s: %v", ErrInvalid, dir, err)
	}
	return sk, nil
}
