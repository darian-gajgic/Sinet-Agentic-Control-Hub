package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The onboarding scan (Spec S13.7). DETERMINISTIC platform code at v0 — no
// model duty (S13.7 names none; S06.2 deterministic-first). Toolchain/config
// heuristics propose commands (build/test/lint/run/preview), obvious-hazard
// heuristics + defaults propose danger zones, and marker files propose
// conventions. The owner edits/extends every field at the approval card
// (Approve's edited draft); the scan only proposes.

// scanRule maps a marker file to a proposed command set (the first matching
// toolchain wins; more specific package managers are checked before their
// fallbacks).
type toolchainRule struct {
	marker  string // a file whose presence identifies the toolchain
	build   string
	test    string
	lint    string
	run     string
	preview string
	conv    string // a convention statement the marker implies
}

// toolchainRules are ordered most-specific-first (Spec S13.8 preview
// providers cribbed from Railpack: package.json → pnpm/npm, pyproject → uv,
// mise.toml → mise, index.html → static).
var toolchainRules = []toolchainRule{
	{marker: "go.mod", build: "go build ./...", test: "go test ./...", lint: "go vet ./...", run: "go run .", conv: "Go project: gofmt-clean, go vet-clean"},
	{marker: "Cargo.toml", build: "cargo build", test: "cargo test", lint: "cargo clippy", run: "cargo run", conv: "Rust project: cargo fmt / clippy"},
	{marker: "pyproject.toml", test: "pytest", lint: "ruff check .", run: "python -m app", conv: "Python project: uv-managed"},
	{marker: "package.json", build: "npm run build", test: "npm test", lint: "npm run lint", run: "npm start", preview: "npm run preview", conv: "Node project"},
	{marker: "Makefile", build: "make", test: "make test", lint: "make lint", run: "make run"},
	{marker: "index.html", preview: "static server", conv: "Static site"},
}

// dangerRule maps a hazardous path to a danger-zone rule. A file that exists
// becomes a danger zone with the guarded content's source hash.
type dangerRule struct {
	path string
	dir  bool // path is a directory (hash its sorted listing)
	rule string
}

var dangerRules = []dangerRule{
	{path: ".env", rule: "secrets file — never read, modify, or commit"},
	{path: ".env.local", rule: "local secrets — never read, modify, or commit"},
	{path: ".env.production", rule: "production secrets — never read, modify, or commit"},
	{path: "secrets.yaml", rule: "secrets file — never read, modify, or commit"},
	{path: "id_rsa", rule: "private key — never read, modify, or commit"},
	{path: ".github/workflows", dir: true, rule: "CI configuration — changes are high-stakes, require approval"},
	{path: "migrations", dir: true, rule: "schema migrations are immutable once committed"},
	{path: "package-lock.json", rule: "lockfile — regenerate via the tool, never hand-edit"},
	{path: "pnpm-lock.yaml", rule: "lockfile — regenerate via the tool, never hand-edit"},
	{path: "Cargo.lock", rule: "lockfile — regenerate via the tool, never hand-edit"},
	{path: "Dockerfile", rule: "container/deploy config — changes affect production"},
	{path: "docker-compose.yml", rule: "deploy config — changes affect production"},
}

// Scan reads a project store and proposes a draft capture (Spec S13.7). It is
// pure over the filesystem state and safe on any directory (a git store, a
// worktree, or a plain dir); .git is never inspected. defaultBranch names the
// project's default branch so the force-push danger zone pins the REAL ref
// (never a placeholder that would flow into ledger §2).
func (s *Store) Scan(store, defaultBranch string) (Draft, error) {
	var d Draft
	present := func(rel string) (string, bool) {
		p := filepath.Join(store, rel)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		return "", false
	}

	// Commands + toolchain conventions: first matching toolchain wins.
	for _, r := range toolchainRules {
		if _, ok := present(r.marker); !ok {
			continue
		}
		d.Commands = Commands{Build: r.build, Test: r.test, Lint: r.lint, Run: r.run, Preview: r.preview}
		if r.marker == "package.json" {
			if _, ok := present("pnpm-lock.yaml"); ok {
				d.Commands = Commands{Build: "pnpm build", Test: "pnpm test", Lint: "pnpm lint", Run: "pnpm start", Preview: "pnpm preview"}
			}
		}
		if r.conv != "" {
			d.Conventions = append(d.Conventions, r.conv)
		}
		break
	}

	// Convention marker files.
	for _, conv := range []struct{ file, note string }{
		{".editorconfig", "Formatting per .editorconfig"},
		{".prettierrc", "Prettier formatting"},
		{"CONTRIBUTING.md", "Follow CONTRIBUTING.md"},
		{"AGENTS.md", "Agent instructions in AGENTS.md apply"},
		{"CLAUDE.md", "Agent instructions in CLAUDE.md apply"},
	} {
		if _, ok := present(conv.file); ok {
			d.Conventions = append(d.Conventions, conv.note)
		}
	}
	d.Conventions = dedupeSorted(d.Conventions)

	// Danger zones, each carrying the guarded content's source hash (the drift
	// input, Spec S05.6 row 2). A default secrets/force-push zone always
	// applies even when no marker file exists.
	for _, r := range dangerRules {
		p := filepath.Join(store, r.path)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		z := DangerZone{Path: r.path, Action: "write", Rule: r.rule}
		if r.dir && info.IsDir() {
			z.SourceHash = dirListingHash(p)
		} else if !r.dir && !info.IsDir() {
			if b, err := os.ReadFile(p); err == nil {
				z.SourceHash = hashContent(b)
			}
		} else {
			continue
		}
		d.DangerZones = append(d.DangerZones, z)
	}
	// The always-on defaults (obvious hazards independent of any file). The
	// force-push zone pins the project's REAL default branch.
	branch := defaultBranch
	if branch == "" {
		branch = "main"
	}
	// The Rule sentences are served VERBATIM on the onboarding approval card
	// (stage/onboard.go), which makes them requester copy: they say what the
	// rule protects in plain words, and their spec citations live here in the
	// comment instead (P3-GF13). The first is D2 (the broker holds every
	// credential; a workspace never does), the second Spec S13.6 (a protected
	// ref only ever gains accepted work — force-push is not a landing verb).
	d.DangerZones = append(d.DangerZones,
		DangerZone{Path: "**/*credential*", Action: "read", Rule: "passwords and access keys never live in a project's files — the platform holds them and hands one over only for the moment a step needs it"},
		DangerZone{Path: branch, Action: "force-push", Rule: "this branch is only ever added to — accepted work lands on it, and nothing rewrites what is already there"},
	)
	sort.SliceStable(d.DangerZones, func(i, j int) bool { return d.DangerZones[i].Path < d.DangerZones[j].Path })

	d.ScanHash = scanHash(d)
	return d, nil
}

// scanHash fingerprints the drafted capture's drift-relevant content (the
// commands, conventions, and every danger zone's guarded source hash) so a
// re-scan detects change deterministically (Spec S13.7).
func scanHash(d Draft) string {
	var sb strings.Builder
	sb.WriteString("commands\x00")
	sb.WriteString(d.Commands.Build + "\x00" + d.Commands.Test + "\x00" + d.Commands.Lint + "\x00" + d.Commands.Run + "\x00" + d.Commands.Preview + "\x00")
	sb.WriteString("conventions\x00")
	for _, c := range d.Conventions {
		sb.WriteString(c + "\x00")
	}
	sb.WriteString("zones\x00")
	zones := append([]DangerZone(nil), d.DangerZones...)
	sort.SliceStable(zones, func(i, j int) bool { return zones[i].Path < zones[j].Path })
	for _, z := range zones {
		sb.WriteString(z.Path + "\x00" + z.Action + "\x00" + z.SourceHash + "\x00")
	}
	return hashContent([]byte(sb.String()))
}

// dirListingHash hashes a directory's sorted top-level listing (a directory
// danger zone's drift baseline).
func dirListingHash(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return hashContent([]byte(strings.Join(names, "\x00")))
}

func dedupeSorted(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(v))
	for _, s := range v {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
