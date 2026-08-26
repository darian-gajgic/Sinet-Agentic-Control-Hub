package stage

// lanepin_ln9r1_map_test.go — P3-LN-9 drain r1 F1/F2, the mapper's own contract.
//
// In-package because mapIntakeErr is unexported, and asserted HERE rather than
// through a production verb because the mapping table IS the unit of this fix.
// The behaviour it serves is pinned through the live paths in
// lanepin_ln9r1_test.go; this is the table itself.
//
// The gap it closes: the BOUNDARY's refusal was mapped from the first cut,
// SELECTION's was not. So a pin that reached layer 3 by any route the boundary
// does not front escaped unmapped and reached the transport as a bare 500 — a
// platform defect's status on a bad request, which tells the requester nothing
// about what to fix (§30: an unhonorable pin is a 4xx, never a 500).

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

func TestLN9R1BothLanePinSentinelsMapToOneCode(t *testing.T) {
	for name, err := range map[string]error{
		// Wrapped, the way each actually arrives — a bare sentinel would test
		// a shape the pipeline never produces.
		"the boundary's refusal": fmt.Errorf("%w: lane %q is not one this platform can pin a task to", intake.ErrLanePinRefused, "zai"),
		"selection's refusal":    fmt.Errorf("intake: route selection: %w: lane %q has no execution seat", worker.ErrLanePinUnhonorable, "zai"),
	} {
		mapped := mapIntakeErr(err)
		var se *api.SurfaceError
		if !errors.As(mapped, &se) {
			t.Fatalf("%s escaped UNMAPPED as %T (%v) — an unmapped domain error reaches the transport as a "+
				"bare 500", name, mapped, mapped)
		}
		if se.Status != http.StatusBadRequest {
			t.Errorf("%s mapped to %d, want 400 — an unhonorable pin is a bad REQUEST, never a platform "+
				"defect (§30)", name, se.Status)
		}
		if se.Code != "lane_pin_refused" {
			t.Errorf("%s mapped to code %q, want lane_pin_refused — ONE code for every unhonorable pin, "+
				"whichever layer raised it (OQ-5)", name, se.Code)
		}
		if se.Msg == "" {
			t.Errorf("%s mapped to an empty message", name)
		}
	}
}
