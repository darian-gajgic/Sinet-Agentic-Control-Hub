// Command lockgate is the components.lock gate of Spec S01.11 / S16.2,
// runnable locally and as the CI lock-gate step:
//
//	go run ./tools/lockgate [-repo <path>]
//
// It validates the manifest against the S16.2 field rules, asserts that
// every Go module dependency is covered by a lock entry, that the go.mod go
// directive matches the Go-toolchain pin, and that every workflow action is
// SHA-pinned and covered by a lock entry.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/lockfile"
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

	summary = fmt.Sprintf("lockgate: OK — %d entries; %d go.mod dependencies covered; %d workflow action references pinned and covered",
		len(lock.Components), len(requires), len(uses))
	return problems, summary
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
