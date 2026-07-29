package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The npm half of the adoption rail (S16.2's "CI fails when any running unit or
// bundled dependency lacks an entry", applied to the SPA tree). It is the
// npm-side equivalent of the Go `modules` coverage rule, with one difference
// the ecosystem forces: a frontend entry claims its ROOT package names and the
// gate COMPUTES the closure from the lockfile's own graph. The four picks pull
// in ~120 production packages between them; enumerating those per entry would
// be regenerated noise that buries the one addition worth reviewing.
//
// Graph semantics (recorded in P3/CONVENTIONS.md §41 — the gate's graph is the
// INSTALLER's graph, so a legitimate transitive never fails the gate):
//
//   - Edges followed: dependencies, optionalDependencies, peerDependencies.
//     npm 7+ installs missing peers and platform-specific optional binaries as
//     real nodes, so both are genuine install edges. devDependencies are
//     followed from nothing: only the project root has them, and its dev tree
//     is toolchain-scoped by classification rather than by walking.
//   - Resolution: npm's own nearest-node_modules walk-up from the importer.
//   - Classification: npm's `dev`/`devOptional` flags mark everything reachable
//     only through the root's devDependencies. Those are toolchain-scoped
//     wholesale under the tree entry. Everything else is the production closure
//     and must be reachable from a claimed root.
const (
	// NPMTreeName is the entry whose pin identifies the WHOLE SPA dependency
	// tree (S16.3 "React 19 + Vite + TypeScript tree"). Its pin is a tree
	// identity, not a package version, so it is exempt from the per-package pin
	// lockstep below and carries its own content-hash check instead.
	NPMTreeName = "React 19 + Vite + TypeScript tree"

	// npmLockfileVersion is the package-lock format this gate reads. A bump is
	// a deliberate edit here, never a silent reinterpretation of the tree.
	npmLockfileVersion = 3

	npmModulesDir = "node_modules/"
)

// NPMLock is the parsed web/package-lock.json.
//
// Unknown fields are tolerated, unlike components.lock: this file is npm's
// format, not ours, and every npm release may add keys. What the gate reads is
// pinned by the lockfileVersion check.
type NPMLock struct {
	Name            string                `json:"name"`
	LockfileVersion int                   `json:"lockfileVersion"`
	Packages        map[string]NPMPackage `json:"packages"`
}

// NPMPackage is one installed node, keyed in Packages by its install path
// ("" is the project itself, "node_modules/x" a top-level install).
type NPMPackage struct {
	Version              string            `json:"version"`
	Dev                  bool              `json:"dev"`
	DevOptional          bool              `json:"devOptional"`
	Optional             bool              `json:"optional"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
}

// toolchainScoped reports whether npm placed this package in the project's
// devDependency tree — the build toolchain, covered wholesale by the tree entry.
func (p NPMPackage) toolchainScoped() bool { return p.Dev || p.DevOptional }

// NPMManifest is the part of web/package.json the gate reads.
type NPMManifest struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// ParseNPMLock decodes web/package-lock.json.
func ParseNPMLock(data []byte) (*NPMLock, error) {
	var l NPMLock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("web/package-lock.json: %w", err)
	}
	return &l, nil
}

// ParseNPMManifest decodes web/package.json.
func ParseNPMManifest(data []byte) (*NPMManifest, error) {
	var m NPMManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("web/package.json: %w", err)
	}
	return &m, nil
}

// NPMTreePin composes the tree entry's pin from the build tool's version and
// the lockfile bytes: the lockfile IS the resolved tree, so hashing it makes
// the tree pin exact in the same sense every other pin is, and any dependency
// movement becomes a deliberate, dated lock edit (CONVENTIONS §4).
func NPMTreePin(viteVersion string, lockBytes []byte) string {
	sum := sha256.Sum256(lockBytes)
	return viteVersion + "+sha256:" + hex.EncodeToString(sum[:])
}

// CheckNPM asserts the npm half of the rail over the real tree: every installed
// production package is covered by an entry or explicitly toolchain-scoped,
// every frontend pin is in lockstep with what is actually installed and
// declared, and the tree entry's pin matches the lockfile it claims.
//
// lockBytes is the raw package-lock.json (the tree pin hashes it).
func (f *File) CheckNPM(lock *NPMLock, manifest *NPMManifest, lockBytes []byte) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if lock.LockfileVersion != npmLockfileVersion {
		add("web/package-lock.json: lockfileVersion %d, want %d (the gate reads that format)",
			lock.LockfileVersion, npmLockfileVersion)
		return problems
	}
	if len(lock.Packages) == 0 {
		add("web/package-lock.json: no packages — the tree is empty")
		return problems
	}

	problems = append(problems, checkNPMExactDeclarations(manifest)...)

	// Claimed roots, and the entry that claims each.
	roots := map[string]string{}
	var rootPaths []string
	for _, c := range f.Components {
		for _, name := range c.NpmPackages {
			path := npmModulesDir + name
			if _, ok := lock.Packages[path]; !ok {
				add("%s: claims npm package %q, which is not installed in web/package-lock.json", c.Name, name)
				continue
			}
			roots[name] = c.Name
			rootPaths = append(rootPaths, path)
		}
		problems = append(problems, checkNPMPinLockstep(c, lock, manifest)...)
	}
	if len(rootPaths) == 0 {
		add("components.lock: no entry claims any npm package, but web/package-lock.json exists (S16.2)")
		return problems
	}

	problems = append(problems, f.checkNPMTreePin(lock, lockBytes)...)

	// Coverage: every production package must be reachable from a claimed root.
	reached := reachableNPM(lock.Packages, rootPaths)
	var uncovered []string
	for path, pkg := range lock.Packages {
		if path == "" || pkg.toolchainScoped() || reached[path] {
			continue
		}
		uncovered = append(uncovered, path)
	}
	sort.Strings(uncovered)
	for _, path := range uncovered {
		add("web/package-lock.json: %s is installed for production but no components.lock entry covers it "+
			"(claim its root package, or record why it is toolchain-scoped; S16.2)",
			strings.TrimPrefix(path, npmModulesDir))
	}
	return problems
}

// checkNPMExactDeclarations applies the §4 pin discipline to the npm side: a
// direct declaration is an exact version, never a range the resolver may move
// under us.
func checkNPMExactDeclarations(m *NPMManifest) []string {
	var problems []string
	for _, block := range []struct {
		field string
		deps  map[string]string
	}{
		{"dependencies", m.Dependencies},
		{"devDependencies", m.DevDependencies},
	} {
		names := make([]string, 0, len(block.deps))
		for name := range block.deps {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			problems = append(problems, validatePin(
				fmt.Sprintf("web/package.json %s.%s", block.field, name), block.deps[name])...)
		}
	}
	return problems
}

// checkNPMPinLockstep asserts the three statements of a frontend pin agree:
// the lock entry, the version npm actually installed, and the exact version
// package.json declares. A silent drift between them is how a pinned adoption
// stops being pinned.
func checkNPMPinLockstep(c Component, lock *NPMLock, manifest *NPMManifest) []string {
	if len(c.NpmPackages) == 0 || c.Name == NPMTreeName {
		return nil
	}
	var problems []string
	for _, name := range c.NpmPackages {
		pkg, ok := lock.Packages[npmModulesDir+name]
		if !ok {
			continue // already reported as an unclaimable root
		}
		if pkg.Version != c.Pin {
			problems = append(problems, fmt.Sprintf(
				"%s: pin %q but web/package-lock.json installs %s@%s", c.Name, c.Pin, name, pkg.Version))
		}
		declared, ok := manifest.Dependencies[name]
		if !ok {
			declared, ok = manifest.DevDependencies[name]
		}
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s: claims npm package %q, which web/package.json does not declare directly", c.Name, name))
			continue
		}
		if declared != c.Pin {
			problems = append(problems, fmt.Sprintf(
				"%s: pin %q but web/package.json declares %s@%s", c.Name, c.Pin, name, declared))
		}
	}
	return problems
}

// checkNPMTreePin asserts the toolchain tree entry's pin still names this
// lockfile: the build tool's installed version plus the lockfile's content hash.
func (f *File) checkNPMTreePin(lock *NPMLock, lockBytes []byte) []string {
	for _, c := range f.Components {
		if c.Name != NPMTreeName {
			continue
		}
		vite, ok := lock.Packages[npmModulesDir+"vite"]
		if !ok {
			return []string{fmt.Sprintf("%s: vite is not installed in web/package-lock.json", NPMTreeName)}
		}
		if want := NPMTreePin(vite.Version, lockBytes); c.Pin != want {
			return []string{fmt.Sprintf(
				"%s: pin %q but the installed tree is %q — the lockfile moved; re-pin the entry as a dated lock edit (S16.2)",
				NPMTreeName, c.Pin, want)}
		}
		return nil
	}
	return []string{fmt.Sprintf("components.lock: no entry named %q covers the SPA toolchain tree (S16.3)", NPMTreeName)}
}

// reachableNPM walks the installed graph from the claimed roots.
func reachableNPM(pkgs map[string]NPMPackage, roots []string) map[string]bool {
	seen := make(map[string]bool, len(pkgs))
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		path := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[path] {
			continue
		}
		seen[path] = true
		pkg := pkgs[path]
		for _, edges := range []map[string]string{pkg.Dependencies, pkg.OptionalDependencies, pkg.PeerDependencies} {
			for name := range edges {
				if next, ok := resolveNPM(pkgs, path, name); ok && !seen[next] {
					queue = append(queue, next)
				}
			}
		}
	}
	return seen
}

// resolveNPM is npm's own resolution: look in the importer's node_modules, then
// each ancestor's, up to the tree root.
func resolveNPM(pkgs map[string]NPMPackage, from, name string) (string, bool) {
	prefix := from
	for {
		candidate := npmModulesDir + name
		if prefix != "" {
			candidate = prefix + "/" + npmModulesDir + name
		}
		if _, ok := pkgs[candidate]; ok {
			return candidate, true
		}
		if prefix == "" {
			return "", false
		}
		if i := strings.LastIndex(prefix, "/"+npmModulesDir); i >= 0 {
			prefix = prefix[:i]
		} else {
			prefix = ""
		}
	}
}
