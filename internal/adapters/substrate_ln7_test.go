package adapters_test

// substrate_ln7_test.go — P3-LN-7 §10 spec T1 (S03.2, S00.9 A12).
//
// The third substrate constant and its lane, plus the marker site A12 claims
// in this package's own source. The source assertion is not decoration: A12
// names the substrate-const block comment as one of four marker sites, and a
// comment still claiming "exactly two substrates" beside three constants is
// the kind of stale sentence a reader trusts because it is adjacent to code.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

func TestSubstrateConstantsAndA12Marker(t *testing.T) {
	if adapters.SubstrateKimiCLI != "kimi-cli" {
		t.Errorf("SubstrateKimiCLI = %q, want %q — it is a runs.substrate value", adapters.SubstrateKimiCLI, "kimi-cli")
	}
	if adapters.LaneKimiCLI != "kimi-cli" {
		t.Errorf("LaneKimiCLI = %q, want %q — it is a runs.lane value", adapters.LaneKimiCLI, "kimi-cli")
	}
	// The lane and the substrate share a name here and that is deliberate: one
	// engine carries exactly one lane on this substrate. They are still two
	// constants, because the platform reads them in two different columns.
	if adapters.SubstrateKimiCLI != adapters.LaneKimiCLI {
		t.Errorf("substrate %q and lane %q disagree", adapters.SubstrateKimiCLI, adapters.LaneKimiCLI)
	}
	// The three substrates are distinct: a collision would make runs.substrate
	// ambiguous and dispatch would follow whichever map entry won.
	seen := map[string]string{}
	for name, v := range map[string]string{
		"SubstrateClaudeCLI": adapters.SubstrateClaudeCLI,
		"SubstrateOpencode":  adapters.SubstrateOpencode,
		"SubstrateKimiCLI":   adapters.SubstrateKimiCLI,
	} {
		if first, dup := seen[v]; dup {
			t.Errorf("%s and %s both = %q", first, name, v)
		}
		seen[v] = name
	}

	src := readAdaptersSource(t)
	if strings.Contains(src, "exactly two substrates") {
		t.Error(`adapters.go still claims "v0 ships exactly two substrates" beside three substrate constants — A12 names this comment as a marker site`)
	}
	block := substrateConstBlock(t, src)
	for _, needle := range []string{"A12", "three substrates", "kimi-cli"} {
		if !strings.Contains(block, needle) {
			t.Errorf("the substrate const block does not carry %q — A12 claims this marker site:\n%s", needle, block)
		}
	}
	// The lane constant says the thing that makes the comparison honest: the
	// two kimi lanes draw ONE pool. A reader who misses that reads two lanes'
	// consumption as two allowances.
	laneBlock := laneConstBlock(t, src)
	for _, needle := range []string{"LaneKimiCLI", "pool"} {
		if !strings.Contains(laneBlock, needle) {
			t.Errorf("the lane const block does not carry %q — the shared-pool fact belongs where the lane is declared:\n%s", needle, laneBlock)
		}
	}
}

func readAdaptersSource(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("adapters.go"))
	if err != nil {
		t.Fatalf("read adapters.go: %v", err)
	}
	return string(raw)
}

// substrateConstBlock returns the const block declaring the substrate names,
// with the doc comment that precedes it.
func substrateConstBlock(t *testing.T, src string) string {
	t.Helper()
	return blockAround(t, src, "SubstrateClaudeCLI =")
}

func laneConstBlock(t *testing.T, src string) string {
	t.Helper()
	return blockAround(t, src, "LaneAnthropic =")
}

// blockAround returns the `const (` … `)` group containing needle, together
// with the comment lines immediately above the group.
func blockAround(t *testing.T, src, needle string) string {
	t.Helper()
	idx := strings.Index(src, needle)
	if idx < 0 {
		t.Fatalf("adapters.go does not declare %s", needle)
	}
	start := strings.LastIndex(src[:idx], "const (")
	if start < 0 {
		t.Fatalf("%s is not inside a const group", needle)
	}
	// Walk back over the contiguous comment lines above the group.
	head := src[:start]
	lines := strings.Split(head, "\n")
	first := len(lines) - 1
	for first > 0 && strings.HasPrefix(strings.TrimSpace(lines[first-1]), "//") {
		first--
	}
	end := strings.Index(src[start:], "\n)")
	if end < 0 {
		t.Fatalf("the const group containing %s is unterminated", needle)
	}
	return strings.Join(lines[first:], "\n") + src[start:start+end]
}
