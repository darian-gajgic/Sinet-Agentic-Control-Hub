package metering_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
)

// tq5_receipt_test.go — P3-TQ-5 acceptance (Spec S07.11: "the self-family-judge
// flag rides the same receipt [G1 Def.1]"; S10.10). The receipt gains ONE
// additive member, `judge`, composed by the serving side (stage.Surface.Receipt,
// the P3-GF4 OQ5 precedent for `verification`) from the run's keep-forever
// verify.round rows: which model judged, whether it shares the executor's
// family, and the plain-words line a requester reads. metering never imports
// verify: the member is strings and a bool.
//
// RED-BY-COMPILE at grounding: metering.JudgeLine and Receipt.Judge do not
// exist yet.

func TestTQ5ReceiptJudgeLineIsAdditive(t *testing.T) {
	// Absent: a receipt with no judge line serves exactly the bytes it served
	// before — no "judge" key at all.
	plain, err := json.Marshal(metering.Receipt{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), `"judge"`) {
		t.Fatalf("an empty receipt grew a judge key: %s", plain)
	}

	r := metering.Receipt{Judge: &metering.JudgeLine{
		Model:      "claude-opus-5",
		SelfFamily: true,
		Note:       "The work and the check of the work were both done by the same model family (claude-opus-5 did both), so this check is less independent than usual.",
	}}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	j, ok := m["judge"].(map[string]any)
	if !ok {
		t.Fatalf("no judge object on the wire: %s", b)
	}
	if j["model"] != "claude-opus-5" || j["self_family"] != true {
		t.Errorf("judge line = %v, want model claude-opus-5 and self_family true", j)
	}
	note, _ := j["note"].(string)
	if note == "" || strings.Contains(note, "S07") || strings.Contains(note, "G1 Def") {
		t.Errorf("the note is requester copy — present and free of spec citations: %q", note)
	}
	var back metering.Receipt
	if err := json.Unmarshal(b, &back); err != nil || back.Judge == nil || *back.Judge != *r.Judge {
		t.Errorf("judge line does not round-trip: %+v (%v)", back.Judge, err)
	}
}
