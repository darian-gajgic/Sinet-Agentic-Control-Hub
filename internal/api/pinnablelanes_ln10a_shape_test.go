package api_test

// pinnablelanes_ln10a_shape_test.go — P3-LN-10a, the served ROW (brief R4, R5).
//
// The rows are produced by the REAL producer — worker.PinnableLanes over a
// worker.Coverage — and field-copied onto the seam type the way the composition
// root does (stage.lanePinOptions). The refusal SENTENCE is never typed by hand
// anywhere here: it is compared against worker.LanePinRefusal, because what
// this read is for is handing the picker the words the boundary will refuse a
// submission with, not a paraphrase of them.
//
// $0: no provider call on any path — Coverage is a value, and nothing dispatches.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln10aCoverage is the three-flat-rate-lane world with the local engine lane:
// the configured lane first, two commissioned lanes after it, local last —
// the order stage.New composes (the configured lane is prepended).
func ln10aCoverage() worker.Coverage {
	return worker.Coverage{
		FlatRateLanes: []string{adapters.LaneAnthropic, adapters.LaneKimi, adapters.LaneZAI},
		LocalLane:     adapters.LaneLocal,
	}
}

// ln10aOptions adapts the producer's verdicts to the intake seam type exactly
// as the composition root does — three fields, no judgment.
func ln10aOptions(cov worker.Coverage) []intake.LanePinOption {
	pinnable := worker.PinnableLanes(cov)
	out := make([]intake.LanePinOption, 0, len(pinnable))
	for _, p := range pinnable {
		out = append(out, intake.LanePinOption{Lane: p.Lane, Pinnable: p.Pinnable, NotPinnable: p.NotPinnable})
	}
	return out
}

func ln10aServe(t *testing.T, opts []intake.LanePinOption) []map[string]json.RawMessage {
	t.Helper()
	b := newBackend(t)
	srv := api.New(api.Config{
		Log:           b.log,
		Sessions:      b.store,
		DevPosture:    true,
		Auth:          staticAuth{id: api.Identity{UserID: "alice"}},
		Settings:      fixedSettings{d: 20 * 1e9},
		HealthFn:      func() api.Health { return api.Health{Ready: true} },
		DB:            b.db,
		Intake:        &fakeSurface{},
		PinnableLanes: opts,
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", pinnableLanesPath, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d (want 200): %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Lanes []map[string]json.RawMessage `json:"lanes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the object envelope: %v\n%s", err, rr.Body.String())
	}
	return body.Lanes
}

// TestLN10aRowShapeCarriesBothKinds — brief R4.
//
// Three pinnable rows and the local lane's refusal, with the key set pinned on
// BOTH kinds: `not_pinnable` is absent from a pinnable row rather than present
// and empty, so nobody has to decide what an empty reason means.
func TestLN10aRowShapeCarriesBothKinds(t *testing.T) {
	cov := ln10aCoverage()
	rows := ln10aServe(t, ln10aOptions(cov))
	if len(rows) != 4 {
		t.Fatalf("served %d rows, want 4 (three flat-rate lanes + the local engine lane): %v", len(rows), rows)
	}

	for i, want := range []string{adapters.LaneAnthropic, adapters.LaneKimi, adapters.LaneZAI} {
		row := rows[i]
		if got := string(row["lane"]); got != `"`+want+`"` {
			t.Errorf("row %d lane = %s, want %q", i, got, want)
		}
		if got := string(row["pinnable"]); got != "true" {
			t.Errorf("row %d (%s) pinnable = %s, want true", i, want, got)
		}
		if raw, ok := row["not_pinnable"]; ok {
			t.Errorf("row %d (%s) carries not_pinnable = %s — omitempty must keep the key OFF a pinnable row",
				i, want, raw)
		}
		if len(row) != 2 {
			t.Errorf("row %d (%s) has keys %v, want exactly lane + pinnable", i, want, keysOf(row))
		}
	}

	local := rows[3]
	if got := string(local["lane"]); got != `"`+adapters.LaneLocal+`"` {
		t.Errorf("last row lane = %s, want %q — the local engine lane is appended last", got, adapters.LaneLocal)
	}
	if got := string(local["pinnable"]); got != "false" {
		t.Errorf("local row pinnable = %s, want false", got)
	}
	if len(local) != 3 {
		t.Errorf("local row has keys %v, want lane + pinnable + not_pinnable", keysOf(local))
	}
	// The SENTENCE is the selection layer's, carried verbatim: the same words
	// refuseLanePin quotes when it refuses a submission naming this lane. A
	// paraphrase here would be the second spelling §65 forbids.
	want := worker.LanePinRefusal(cov, adapters.LaneLocal)
	if want == "" {
		t.Fatal("worker.LanePinRefusal returned no sentence for the local lane — the comparison below would be vacuous")
	}
	var got string
	if err := json.Unmarshal(local["not_pinnable"], &got); err != nil {
		t.Fatalf("not_pinnable is not a JSON string: %v", err)
	}
	if got != want {
		t.Errorf("not_pinnable was composed or trimmed by the transport.\n got: %s\nwant: %s", got, want)
	}
}

// TestLN10aOrderIsTheComposedOrder — brief R5.
//
// The composition owns the order and it is meaningful: the platform's own
// flat-rate lane first, local last. Nothing here sorts. The mutation is what
// makes that a claim rather than a coincidence — feeding the SAME rows reversed
// must flip the served body, which a sort would hide.
func TestLN10aOrderIsTheComposedOrder(t *testing.T) {
	opts := ln10aOptions(ln10aCoverage())
	forward := laneNamesOf(ln10aServe(t, opts))

	reversed := make([]intake.LanePinOption, 0, len(opts))
	for i := len(opts) - 1; i >= 0; i-- {
		reversed = append(reversed, opts[i])
	}
	back := laneNamesOf(ln10aServe(t, reversed))

	if reflect.DeepEqual(forward, back) {
		t.Fatalf("reversing the composed rows served the same order %v — the transport is sorting, "+
			"which is a second spelling of an order the composition owns", forward)
	}
	for i := range forward {
		if back[i] != forward[len(forward)-1-i] {
			t.Fatalf("served order %v is not the reversed input — the rows are being reordered, not carried", back)
		}
	}
}

// TestLN10aServingDoesNotMutateTheSharedSet — brief R5.
//
// The Config slice is the SKELETON's own, shared with the boundary that refuses
// against it. A handler that sorted or rewrote it in place would change what
// submissions the platform accepts, from a read.
func TestLN10aServingDoesNotMutateTheSharedSet(t *testing.T) {
	opts := ln10aOptions(ln10aCoverage())
	before := append([]intake.LanePinOption(nil), opts...)
	ln10aServe(t, opts)
	ln10aServe(t, opts)
	if !reflect.DeepEqual(opts, before) {
		t.Errorf("serving the read mutated the shared lane-pin set.\n got: %+v\nwant: %+v", opts, before)
	}
}

func keysOf(row map[string]json.RawMessage) []string {
	out := make([]string, 0, len(row))
	for k := range row {
		out = append(out, k)
	}
	return out
}

func laneNamesOf(rows []map[string]json.RawMessage) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var lane string
		_ = json.Unmarshal(row["lane"], &lane)
		out = append(out, lane)
	}
	return out
}
