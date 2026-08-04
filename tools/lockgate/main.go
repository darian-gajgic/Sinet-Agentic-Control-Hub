// Command lockgate is the components.lock gate of Spec S01.11 / S16.2,
// runnable locally and as the CI lock-gate step:
//
//	go run ./tools/lockgate [-repo <path>]
//
// It validates the manifest against the S16.2 field rules, asserts that
// every Go module dependency is covered by a lock entry, that the go.mod go
// directive matches the Go-toolchain pin, that every workflow action is
// SHA-pinned and covered by a lock entry, and that every npm package installed
// for the SPA is covered by an entry or toolchain-scoped, with the frontend
// pins in lockstep across entry, lockfile and package.json.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/lockfile"
)

func main() {
	repo := flag.String("repo", ".", "path to the repository root")
	flag.Parse()

	problems, summary := run(*repo)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "lockgate: %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "lockgate: FAIL — %d problem(s)\n", len(problems))
		os.Exit(1)
	}
	fmt.Println(summary)
}

func run(repo string) (problems []string, summary string) {
	data, err := os.ReadFile(filepath.Join(repo, "components.lock"))
	if err != nil {
		return []string{err.Error()}, ""
	}
	lock, err := lockfile.Parse(data)
	if err != nil {
		return []string{err.Error()}, ""
	}
	problems = append(problems, lock.Validate()...)

	goVersion, requires, err := readGoMod(repo)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = append(problems, lock.CheckGoDirective(goVersion)...)
		problems = append(problems, lock.CheckGoModules(requires)...)
	}

	uses, err := scanWorkflows(repo)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = append(problems, lock.CheckWorkflowUses(uses)...)
	}

	npmLock, npmManifest, npmBytes, err := readNPM(repo)
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		problems = append(problems, lock.CheckNPM(npmLock, npmManifest, npmBytes)...)
	}

	npmPackages := 0
	if npmLock != nil {
		npmPackages = len(npmLock.Packages) - 1 // the project itself is not a dependency
	}
	summary = fmt.Sprintf("lockgate: OK — %d entries; %d go.mod dependencies covered; %d workflow action references pinned and covered; %d npm packages covered or toolchain-scoped",
		len(lock.Components), len(requires), len(uses), npmPackages)
	return problems, summary
}

// readNPM loads the SPA tree's manifest and lockfile. Their absence is a
// failure, not a skip: components.lock entries claim npm packages, so the tree
// they claim has to exist (S16.2).
func readNPM(repo string) (*lockfile.NPMLock, *lockfile.NPMManifest, []byte, error) {
	lockBytes, err := os.ReadFile(filepath.Join(repo, "web", "package-lock.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("web/package-lock.json: %w", err)
	}
	npmLock, err := lockfile.ParseNPMLock(lockBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(repo, "web", "package.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("web/package.json: %w", err)
	}
	manifest, err := lockfile.ParseNPMManifest(manifestBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	return npmLock, manifest, lockBytes, nil
}

// readGoMod reads the go directive and require paths via `go mod edit
// -json`, so parsing rides the toolchain instead of a third-party module.
func readGoMod(repo string) (goVersion string, requires []string, err error) {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", nil, fmt.Errorf("go mod edit -json: %w", err)
	}
	var mod struct {
		Go      string
		Require []struct{ Path string }
	}
	if err := json.Unmarshal(out, &mod); err != nil {
		return "", nil, fmt.Errorf("go mod edit -json: %w", err)
	}
	for _, r := range mod.Require {
		requires = append(requires, r.Path)
	}
	return mod.Go, requires, nil
}

func scanWorkflows(repo string) ([]lockfile.Use, error) {
	var uses []lockfile.Use
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		paths, err := filepath.Glob(filepath.Join(repo, ".github", "workflows", pattern))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			rel, err := filepath.Rel(repo, path)
			if err != nil {
				rel = path
			}
			uses = append(uses, lockfile.ScanWorkflowUses(rel, content)...)
		}
	}
	return uses, nil
}
