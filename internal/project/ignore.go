package project

import (
	"os"
	"path/filepath"
	"strings"
)

// Platform ignore rules (Spec S13.5: snapshot commits are "junk-excluded via
// platform ignore rules"). These are STRUCTURAL code data, not a ⚙ setting —
// S13.5 ratifies no key for them (the CONVENTIONS §7 sseBatchSize precedent),
// flagged to the B4 gate under the standing settings-tab directive. They are
// applied to a snapshot's `git add -A` through a platform-owned excludes file
// (core.excludesFile) so they never touch the repository's tracked
// .gitignore and never leak into an accepted commit.
//
// The set is deliberately conservative: transient build/dependency output and
// editor/OS scratch that a review would never want, and never source. A
// project's own tracked .gitignore still applies on top (git unions them).
var platformIgnorePatterns = []string{
	"# Sinet platform snapshot excludes (Spec S13.5) — structural, not tracked.",
	".DS_Store",
	"Thumbs.db",
	"*.swp",
	"*.swo",
	"*~",
	"*.tmp",
	"*.log",
	".#*",
	"node_modules/",
	".venv/",
	"venv/",
	"__pycache__/",
	"*.pyc",
	"target/",
	"dist/",
	"build/",
	".gradle/",
	".next/",
	".cache/",
	".pytest_cache/",
	".mypy_cache/",
	".ruff_cache/",
	"*.o",
	"*.a",
	"*.class",
}

// writeExcludesFile materialises the platform ignore rules into an excludes
// file under the projects root, returning its path. Written once per store;
// the content is structural so a rewrite is idempotent.
func (s *Store) writeExcludesFile() (string, error) {
	path := filepath.Join(s.root, "platform-excludes")
	body := strings.Join(platformIgnorePatterns, "\n") + "\n"
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
