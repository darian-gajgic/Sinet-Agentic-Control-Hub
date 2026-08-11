package stage

// forksuffix_internal_test.go — P3-RW-6 §7 property test: the ONE fork-aware
// suffix matcher (OQ5).
//
// Three implementations of "strip the recovery-fork suffix" existed: this
// package's role loop, `metering.StripForkSuffix`, and the shell's raw
// HasSuffix on the workspace gate — and the third one was wrong. The invariant
// is spec-stated (Spec S02.5 step 3: a successor run id is `<parent>.g<newGen>`,
// so a fork is the SAME work under a new identity), so this sweeps the SHAPES
// rather than a handful of examples: for any base id and any chain of fork
// suffixes, the role is unchanged, the strip is exact, and the workspace gate
// (which matches the execute role through the shared matcher) agrees with the
// role the dispatcher would route.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
)

func TestForkSuffixChainAgreement(t *testing.T) {
	// Base ids that are NOT themselves fork-suffixed, incl. digit-bearing and
	// adversarial segments (a task literally named `t-g1`, a `.gadget` segment).
	bases := []string{
		"t-x", "t-9", "t-g1", "t-9.g", "onboard-shop", "bench-1.pair-2",
		"t-x.gadget", "t-deadbeef.gx1",
	}
	roles := []string{RunSuffixIntake, RunSuffixExecute, RunSuffixVerify,
		RunSuffixCompose, RunSuffixOnboard, RunSuffixDirect, ""}
	chains := []string{"", ".g1", ".g2", ".g12", ".g4.g5.g6", ".g0", ".g99.g100"}

	for _, base := range bases {
		for _, rl := range roles {
			id := base + rl
			wantRole, wantRoutable := runRole(id)
			for _, chain := range chains {
				forked := id + chain

				// (1) A fork routes exactly as its base does.
				gotRole, gotRoutable := runRole(forked)
				if gotRoutable != wantRoutable || gotRole != wantRole {
					t.Fatalf("runRole(%q) = (%q,%v), want the base %q's (%q,%v)",
						forked, gotRole, gotRoutable, id, wantRole, wantRoutable)
				}

				// (2) The shared matcher strips exactly the chain — no more.
				if got := metering.StripForkSuffix(forked); got != id {
					t.Fatalf("StripForkSuffix(%q) = %q, want %q", forked, got, id)
				}

				// (3) The workspace gate (the shell's execute-role test, built on
				// the shared matcher) agrees with the dispatcher's role.
				gate := strings.HasSuffix(metering.StripForkSuffix(forked), RunSuffixExecute)
				if want := gotRoutable && gotRole == roleExecute; gate != want {
					t.Fatalf("workspace gate(%q) = %v, want %v (role %q)", forked, gate, want, gotRole)
				}
			}
		}
	}

	// Shapes that are NOT fork suffixes must survive untouched by every reader.
	for _, id := range []string{
		"t-x.execute.g", "t-x.execute.gx", "t-x.execute.g1x", "t-x.execute.g1.foo",
		"t-x.gadget.execute", "t-x.g2.execute",
	} {
		if got := metering.StripForkSuffix(id); got != strippedByRoleLoop(id) {
			t.Fatalf("StripForkSuffix(%q) = %q, disagrees with the role loop's %q",
				id, got, strippedByRoleLoop(id))
		}
	}
}

// strippedByRoleLoop is the role loop's stripping semantics stated
// independently, so the test compares two readings rather than one with itself.
func strippedByRoleLoop(id string) string {
	for {
		i := strings.LastIndexByte(id, '.')
		if i < 0 {
			return id
		}
		seg := id[i+1:]
		if len(seg) < 2 || seg[0] != 'g' {
			return id
		}
		for _, c := range seg[1:] {
			if c < '0' || c > '9' {
				return id
			}
		}
		id = id[:i]
	}
}
