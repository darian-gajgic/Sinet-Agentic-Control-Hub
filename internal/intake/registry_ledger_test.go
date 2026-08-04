package intake_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// TestRegistryDangerZonesPinnedInLedger proves the S13.7 registry feed into
// the S05.1 §2 pinned constraints via the EXISTING piping (Spec S06.9,
// intake/answer.go → ledger.SetConstraints), exercised with registry-shaped
// danger zones carrying source hashes (the ledger §2 pin: path/action + rule +
// source hash). The production registry (repo_registry) supplies exactly this
// RegistrySlice shape (rubric item 5).
func TestRegistryDangerZonesPinnedInLedger(t *testing.T) {
	f := newFix(t)
	f.p.Registry = &fakeRegistry{ok: true, slice: intake.RegistrySlice{
		Project: "shop", Ref: "shop@capture-v1",
		Conventions: []string{"Go project: gofmt-clean"},
		Commands:    []string{"test: go test ./..."},
		DangerZones: []intake.DangerZone{
			{Path: ".env", Rule: "write: secrets file — never modify", SourceHash: "abc123"},
			{Path: ".github/workflows", Rule: "write: CI config — high stakes", SourceHash: "def456"},
		},
	}}

	st, askID, _ := approvalCard(t, f)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase = %s, want approved", st.Phase)
	}

	doc := f.ledgerDoc(st.TaskID)
	byPath := map[string]string{}
	for _, z := range doc.Constraints.DangerZones {
		byPath[z.Path] = z.SourceHash
	}
	if byPath[".env"] != "abc123" {
		t.Fatalf("registry .env danger zone not pinned with its source hash in ledger §2: %+v", doc.Constraints.DangerZones)
	}
	if byPath[".github/workflows"] != "def456" {
		t.Fatalf("registry CI danger zone not pinned in ledger §2: %+v", doc.Constraints.DangerZones)
	}
}
