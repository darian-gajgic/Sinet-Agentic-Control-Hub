// Package lockfile parses and validates components.lock, the adoption
// manifest bound by Spec S16.
//
// S16.2 fixes the entry fields as normative and leaves the serialization to
// P3; the recorded choice (strict, pretty-printed JSON) and the extension
// fields are documented in P3/CONVENTIONS.md. This package is the local half
// of the CI lock-gate (S01.11): tools/lockgate wires it to the repository.
package lockfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kinds enumerates the valid entry kinds, verbatim from S16.2.
var Kinds = []string{
	"engine",
	"organ-unit",
	"library",
	"data",
	"frontend",
	"toolchain",
	"os-mechanism",
	"host-managed",
	"mechanism",
	"standby",
}

// GoToolchainName is the canonical entry name whose pin must match the go
// directive in go.mod (S16.3 Go-toolchain row).
const GoToolchainName = "Go toolchain"

// File is the parsed components.lock.
type File struct {
	Schema     int         `json:"schema"`
	GovernedBy string      `json:"governed_by"`
	Components []Component `json:"components"`
}

// Component is one adoption entry. The first nine fields are the S16.2
// normative set; Modules and Notes are the documented extension fields.
type Component struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Pin         string   `json:"pin"`
	License     License  `json:"license"`
	Role        Role     `json:"role"`
	Replacement string   `json:"replacement"`
	Abandonment string   `json:"abandonment"`
	Watch       []string `json:"watch"`
	LastReview  string   `json:"last_review"`
	Modules     []string `json:"modules,omitempty"`
	// NpmPackages is the npm-side equivalent of Modules (P3/CONVENTIONS.md §4
	// defers its definition to B6, §41 records it): the ROOT package names this
	// entry claims in web/package-lock.json. Transitive coverage is COMPUTED
	// from the lockfile's own dependency graph (npm.go), never enumerated here
	// — the four frontend picks pull in hundreds of names, and a hand-listed
	// closure would be regenerated review noise that hides the one addition
	// that matters.
	NpmPackages []string `json:"npm_packages,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

// License is the S16.2 license field: SPDX id, the path scope actually
// consumed, and the license-check date (P-T16-2).
type License struct {
	SPDX    string `json:"spdx"`
	Scope   string `json:"scope"`
	Checked string `json:"checked"`
}

// Role is the S16.2 role field: one line plus the owning spec section.
type Role struct {
	Summary string `json:"summary"`
	Section string `json:"section"`
}

// Parse decodes a components.lock document. Unknown fields are rejected so
// the manifest schema cannot drift silently.
func Parse(data []byte) (*File, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("components.lock: %w", err)
	}
	if err := dec.Decode(new(json.RawMessage)); err == nil {
		return nil, fmt.Errorf("components.lock: trailing data after document")
	}
	return &f, nil
}

var floatingPinPrefixes = []string{"^", "~", ">", "<", "="}

// Validate checks the manifest against the S16.2 field rules and returns one
// problem string per violation. An empty slice means the manifest is valid.
func (f *File) Validate() []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if f.Schema != 1 {
		add("schema: got %d, want 1", f.Schema)
	}
	if len(f.Components) == 0 {
		add("components: manifest has no entries")
	}

	kinds := make(map[string]bool, len(Kinds))
	for _, k := range Kinds {
		kinds[k] = true
	}
	seenNames := make(map[string]bool)
	seenModules := make(map[string]string)
	seenNpm := make(map[string]string)

	for i, c := range f.Components {
		id := c.Name
		if id == "" {
			id = fmt.Sprintf("components[%d]", i)
			add("%s: name is empty", id)
		}
		if seenNames[c.Name] && c.Name != "" {
			add("%s: duplicate entry name", id)
		}
		seenNames[c.Name] = true

		if !kinds[c.Kind] {
			add("%s: kind %q is not one of %s", id, c.Kind, strings.Join(Kinds, "|"))
		}
		problems = append(problems, validatePin(id, c.Pin)...)

		if c.License.SPDX == "" {
			add("%s: license.spdx is empty", id)
		}
		if c.License.Scope == "" {
			add("%s: license.scope is empty (path scope is mandatory, P-T16-2)", id)
		}
		if !validDate(c.License.Checked) {
			add("%s: license.checked %q is not a YYYY-MM-DD date", id, c.License.Checked)
		}
		if c.Role.Summary == "" {
			add("%s: role.summary is empty", id)
		}
		if c.Role.Section == "" {
			add("%s: role.section is empty", id)
		}
		if c.Replacement == "" {
			add("%s: replacement (funeral plan, part 1) is empty", id)
		}
		if c.Abandonment == "" {
			add("%s: abandonment (funeral plan, part 2) is empty", id)
		}
		if len(c.Watch) == 0 {
			add("%s: watch has no rows", id)
		}
		for _, w := range c.Watch {
			if strings.TrimSpace(w) == "" {
				add("%s: watch contains an empty row", id)
			}
		}
		if !validDate(c.LastReview) {
			add("%s: last_review %q is not a YYYY-MM-DD date", id, c.LastReview)
		}
		for _, m := range c.Modules {
			if strings.TrimSpace(m) == "" {
				add("%s: modules contains an empty path", id)
				continue
			}
			if prev, dup := seenModules[m]; dup {
				add("%s: module %q already claimed by entry %q", id, m, prev)
			}
			seenModules[m] = c.Name
		}
		for _, p := range c.NpmPackages {
			if strings.TrimSpace(p) == "" {
				add("%s: npm_packages contains an empty name", id)
				continue
			}
			if prev, dup := seenNpm[p]; dup {
				add("%s: npm package %q already claimed by entry %q", id, p, prev)
			}
			seenNpm[p] = c.Name
		}
	}
	return problems
}

func validatePin(id, pin string) []string {
	var problems []string
	switch {
	case pin == "":
		problems = append(problems, fmt.Sprintf("%s: pin is empty", id))
	case strings.EqualFold(pin, "latest"), pin == "*":
		problems = append(problems, fmt.Sprintf("%s: pin %q is floating — pins are exact, never latest (S16.2)", id, pin))
	case strings.ContainsAny(pin, " \t"):
		problems = append(problems, fmt.Sprintf("%s: pin %q contains whitespace", id, pin))
	default:
		for _, p := range floatingPinPrefixes {
			if strings.HasPrefix(pin, p) {
				problems = append(problems, fmt.Sprintf("%s: pin %q looks like a version range — pins are exact (S16.2)", id, pin))
				break
			}
		}
	}
	return problems
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// moduleSet returns the union of every entry's Modules list.
func (f *File) moduleSet() map[string]bool {
	set := make(map[string]bool)
	for _, c := range f.Components {
		for _, m := range c.Modules {
			set[m] = true
		}
	}
	return set
}

// CheckGoModules asserts that every required Go module path is covered by
// some entry's modules list — the bundled-dependency half of the S16.2 CI
// rule ("CI fails when any running unit or bundled dependency lacks an
// entry").
func (f *File) CheckGoModules(required []string) []string {
	covered := f.moduleSet()
	var problems []string
	for _, mod := range required {
		if !covered[mod] {
			problems = append(problems, fmt.Sprintf("go.mod requires %s but no components.lock entry covers it (S16.2)", mod))
		}
	}
	return problems
}

// CheckGoDirective asserts that the go directive in go.mod matches the pin
// of the Go-toolchain entry, so the toolchain pin has one source of truth.
func (f *File) CheckGoDirective(goVersion string) []string {
	for _, c := range f.Components {
		if c.Name == GoToolchainName && c.Kind == "toolchain" {
			if c.Pin != goVersion {
				return []string{fmt.Sprintf("go.mod go directive %q != %q pin %q", goVersion, GoToolchainName, c.Pin)}
			}
			return nil
		}
	}
	return []string{fmt.Sprintf("no toolchain entry named %q in components.lock", GoToolchainName)}
}

// Use is one `uses:` reference found in a workflow file.
type Use struct {
	Source string // file the reference came from
	Value  string // e.g. "actions/checkout@<sha>"
}

var (
	usesRe = regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?uses:[ \t]*['"]?([^'"#\s]+)`)
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// ScanWorkflowUses extracts `uses:` action references from workflow YAML.
// It is a deliberate line-level tripwire for this repository's own workflow
// files, not a general YAML parser.
func ScanWorkflowUses(source string, content []byte) []Use {
	var uses []Use
	for _, m := range usesRe.FindAllSubmatch(content, -1) {
		uses = append(uses, Use{Source: source, Value: string(m[1])})
	}
	return uses
}

// CheckWorkflowUses asserts the CI-action half of the adoption rail: every
// action reference is pinned to a full commit SHA and carries a
// components.lock entry named after its owner/repo. Repo-local actions
// ("./...") are exempt; docker:// references are not admitted.
func (f *File) CheckWorkflowUses(uses []Use) []string {
	names := make(map[string]bool, len(f.Components))
	for _, c := range f.Components {
		names[c.Name] = true
	}
	var problems []string
	for _, u := range uses {
		if strings.HasPrefix(u.Value, "./") {
			continue
		}
		if strings.HasPrefix(u.Value, "docker://") {
			problems = append(problems, fmt.Sprintf("%s: uses %q — docker actions are not admitted (no lock-entry mechanism)", u.Source, u.Value))
			continue
		}
		at := strings.LastIndex(u.Value, "@")
		if at < 0 {
			problems = append(problems, fmt.Sprintf("%s: uses %q is unpinned — pin the full commit SHA (S16.2)", u.Source, u.Value))
			continue
		}
		spec, ref := u.Value[:at], u.Value[at+1:]
		if !shaRe.MatchString(ref) {
			problems = append(problems, fmt.Sprintf("%s: uses %q — ref %q is not a full 40-hex commit SHA (S16.2 exact-pin rule)", u.Source, u.Value, ref))
		}
		segs := strings.Split(spec, "/")
		if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
			problems = append(problems, fmt.Sprintf("%s: uses %q — malformed action reference", u.Source, u.Value))
			continue
		}
		ownerRepo := segs[0] + "/" + segs[1]
		if !names[ownerRepo] {
			problems = append(problems, fmt.Sprintf("%s: uses %q but no components.lock entry named %q exists (S16.2)", u.Source, u.Value, ownerRepo))
		}
	}
	return problems
}
