package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// npmDoc builds a components.lock covering a tiny two-pick tree, so each probe
// below can break exactly one thing and see the gate name it.
func npmDoc(t *testing.T, mutate func(*File)) *File {
	t.Helper()
	f := &File{
		Schema:     1,
		GovernedBy: "test",
		Components: []Component{
			{
				Name: "widget", Kind: "frontend", Pin: "2.0.0",
				License:     License{SPDX: "MIT", Scope: "pkg", Checked: "2026-07-29"},
				Role:        Role{Summary: "a widget", Section: "S15.5"},
				Replacement: "another widget", Abandonment: "default",
				Watch: []string{"quarterly"}, LastReview: "2026-07-29",
				NpmPackages: []string{"widget"},
			},
			{
				Name: NPMTreeName, Kind: "toolchain", Pin: "",
				License:     License{SPDX: "MIT", Scope: "tree", Checked: "2026-07-29"},
				Role:        Role{Summary: "the tree", Section: "S15.1"},
				Replacement: "n/a", Abandonment: "default",
				Watch: []string{"quarterly"}, LastReview: "2026-07-29",
				NpmPackages: []string{"react"},
			},
		},
	}
	if mutate != nil {
		mutate(f)
	}
	return f
}

// npmTree builds a lockfile whose production side is react + widget + widget's
// own transitive, plus one dev-only package.
func npmTree(t *testing.T, mutate func(map[string]NPMPackage)) (*NPMLock, *NPMManifest, []byte) {
	t.Helper()
	pkgs := map[string]NPMPackage{
		"": {
			Dependencies:    map[string]string{"react": "19.2.8", "widget": "2.0.0"},
			DevDependencies: map[string]string{"vite": "8.1.5"},
		},
		"node_modules/react":       {Version: "19.2.8"},
		"node_modules/widget":      {Version: "2.0.0", Dependencies: map[string]string{"widget-core": "1.0.0"}, PeerDependencies: map[string]string{"react": "^19"}},
		"node_modules/widget-core": {Version: "1.0.0"},
		"node_modules/vite":        {Version: "8.1.5", Dev: true},
	}
	if mutate != nil {
		mutate(pkgs)
	}
	lock := &NPMLock{Name: "sinet-web", LockfileVersion: 3, Packages: pkgs}
	manifest := &NPMManifest{
		Dependencies:    map[string]string{"react": "19.2.8", "widget": "2.0.0"},
		DevDependencies: map[string]string{"vite": "8.1.5"},
	}
	raw, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	return lock, manifest, raw
}

// check runs the gate with the tree entry's pin already set to whatever the
// lockfile hashes to, so a probe fails for its own reason and not the pin's.
func check(t *testing.T, f *File, lock *NPMLock, m *NPMManifest, raw []byte) []string {
	t.Helper()
	for i := range f.Components {
		if f.Components[i].Name == NPMTreeName && f.Components[i].Pin == "" {
			f.Components[i].Pin = NPMTreePin(lock.Packages["node_modules/vite"].Version, raw)
		}
	}
	return f.CheckNPM(lock, m, raw)
}

func TestNPMGatePassesOnACoveredTree(t *testing.T) {
	lock, m, raw := npmTree(t, nil)
	if problems := check(t, npmDoc(t, nil), lock, m, raw); len(problems) != 0 {
		t.Fatalf("a fully covered tree failed the gate: %v", problems)
	}
}

// (a) The coverage rule: an installed production package no entry reaches.
func TestNPMGateFailsOnAnUncoveredProductionPackage(t *testing.T) {
	lock, m, raw := npmTree(t, func(pkgs map[string]NPMPackage) {
		// Installed for production, depended on by nothing claimed.
		pkgs["node_modules/smuggled"] = NPMPackage{Version: "6.6.6"}
	})

	problems := check(t, npmDoc(t, nil), lock, m, raw)
	if !anyContains(problems, "smuggled") {
		t.Fatalf("an uncovered production package passed the gate: %v", problems)
	}
}

// A transitive of a claimed root is covered WITHOUT being named anywhere: that
// is the whole point of computing the closure instead of enumerating it.
func TestNPMGateCoversTransitivesThroughTheGraph(t *testing.T) {
	lock, m, raw := npmTree(t, func(pkgs map[string]NPMPackage) {
		pkgs["node_modules/widget-core"] = NPMPackage{Version: "1.0.0", Dependencies: map[string]string{"deep": "0.1.0"}}
		pkgs["node_modules/deep"] = NPMPackage{Version: "0.1.0"}
	})

	if problems := check(t, npmDoc(t, nil), lock, m, raw); len(problems) != 0 {
		t.Fatalf("a transitive of a claimed root was reported uncovered: %v", problems)
	}
}

// Dev packages are toolchain-scoped wholesale — no entry claims them one by one.
func TestNPMGateScopesDevPackagesToTheToolchain(t *testing.T) {
	lock, m, raw := npmTree(t, func(pkgs map[string]NPMPackage) {
		pkgs["node_modules/some-dev-only-thing"] = NPMPackage{Version: "1.2.3", Dev: true}
		pkgs["node_modules/a-devoptional-thing"] = NPMPackage{Version: "1.2.3", DevOptional: true}
	})

	if problems := check(t, npmDoc(t, nil), lock, m, raw); len(problems) != 0 {
		t.Fatalf("dev packages were not toolchain-scoped: %v", problems)
	}
}

// (b) Pin lockstep: the entry, the lockfile and package.json must agree.
func TestNPMGateFailsOnPinDrift(t *testing.T) {
	t.Run("lockfile installed a different version", func(t *testing.T) {
		lock, m, raw := npmTree(t, func(pkgs map[string]NPMPackage) {
			p := pkgs["node_modules/widget"]
			p.Version = "2.0.1"
			pkgs["node_modules/widget"] = p
		})
		problems := check(t, npmDoc(t, nil), lock, m, raw)
		if !anyContains(problems, "installs widget@2.0.1") {
			t.Fatalf("a lockfile pin drift passed the gate: %v", problems)
		}
	})

	t.Run("package.json declares a different version", func(t *testing.T) {
		lock, m, raw := npmTree(t, nil)
		m.Dependencies["widget"] = "2.1.0"
		problems := check(t, npmDoc(t, nil), lock, m, raw)
		if !anyContains(problems, "declares widget@2.1.0") {
			t.Fatalf("a package.json pin drift passed the gate: %v", problems)
		}
	})

	t.Run("the entry claims a package nobody installed", func(t *testing.T) {
		lock, m, raw := npmTree(t, nil)
		f := npmDoc(t, func(f *File) { f.Components[0].NpmPackages = []string{"ghost"} })
		problems := check(t, f, lock, m, raw)
		if !anyContains(problems, "not installed") {
			t.Fatalf("a claim on an uninstalled package passed the gate: %v", problems)
		}
	})
}

// The §4 pin discipline reaches the npm side: a direct declaration is exact.
func TestNPMGateFailsOnARangeDeclaration(t *testing.T) {
	lock, m, raw := npmTree(t, nil)
	m.DevDependencies["vite"] = "^8.1.5"

	problems := check(t, npmDoc(t, nil), lock, m, raw)
	if !anyContains(problems, "version range") {
		t.Fatalf("a caret range passed the gate: %v", problems)
	}
}

// The tree pin is the lockfile's identity: move the tree, and the entry has to
// move with it as a deliberate edit.
func TestNPMGateFailsWhenTheTreeMoved(t *testing.T) {
	lock, m, raw := npmTree(t, nil)
	f := npmDoc(t, nil)
	f.Components[1].Pin = NPMTreePin("8.1.5", []byte("a different lockfile"))

	problems := f.CheckNPM(lock, m, raw)
	if !anyContains(problems, "the lockfile moved") {
		t.Fatalf("a stale tree pin passed the gate: %v", problems)
	}
}

func TestNPMGateFailsWithoutATreeEntry(t *testing.T) {
	lock, m, raw := npmTree(t, nil)
	f := npmDoc(t, func(f *File) { f.Components = f.Components[:1] })

	problems := f.CheckNPM(lock, m, raw)
	if !anyContains(problems, "covers the SPA toolchain tree") {
		t.Fatalf("a tree with no toolchain entry passed the gate: %v", problems)
	}
}

// A duplicate claim is caught by field validation, like duplicate Go modules.
func TestNPMDuplicateClaimsRejected(t *testing.T) {
	f := npmDoc(t, func(f *File) { f.Components[1].NpmPackages = []string{"react", "widget"} })

	if problems := f.Validate(); !anyContains(problems, "already claimed by entry") {
		t.Fatalf("two entries claiming one npm package passed validation: %v", problems)
	}
}

// (c) The real files. `go test ./...` trips on npm-manifest breakage before the
// lockgate step ever runs — and the absence of web/ is a failure, not a skip:
// entries claim a tree that has to exist.
func TestRepositoryNPMTreeIsCovered(t *testing.T) {
	root := repoRoot(t)

	lockData, err := os.ReadFile(filepath.Join(root, "components.lock"))
	if err != nil {
		t.Fatalf("read components.lock: %v", err)
	}
	manifest, err := Parse(lockData)
	if err != nil {
		t.Fatal(err)
	}

	npmLockBytes, err := os.ReadFile(filepath.Join(root, "web", "package-lock.json"))
	if err != nil {
		t.Fatalf("read web/package-lock.json: %v", err)
	}
	npmLock, err := ParseNPMLock(npmLockBytes)
	if err != nil {
		t.Fatal(err)
	}
	pkgJSON, err := os.ReadFile(filepath.Join(root, "web", "package.json"))
	if err != nil {
		t.Fatalf("read web/package.json: %v", err)
	}
	npmManifest, err := ParseNPMManifest(pkgJSON)
	if err != nil {
		t.Fatal(err)
	}

	if problems := manifest.CheckNPM(npmLock, npmManifest, npmLockBytes); len(problems) != 0 {
		t.Fatalf("the real SPA tree is not covered by components.lock: %v", problems)
	}

	// The four binding FC-v1 picks are pinned where the spec says, and the
	// claim is on the entry rather than on a comment somewhere.
	for name, pin := range map[string]string{
		"@hello-pangea/dnd":   "18.0.1",
		"react-diff-view":     "3.3.3",
		"@assistant-ui/react": "0.14.27",
		"@jsonforms/core":     "3.8.0",
		"@jsonforms/react":    "3.8.0",
	} {
		pkg, ok := npmLock.Packages["node_modules/"+name]
		if !ok {
			t.Errorf("%s is not installed — FC-v1 binds it", name)
			continue
		}
		if pkg.Version != pin {
			t.Errorf("%s installed at %s, FC-v1 binds %s", name, pkg.Version, pin)
		}
	}
}
