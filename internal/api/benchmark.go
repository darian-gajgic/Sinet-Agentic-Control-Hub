package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// benchmark.go — the BENCH-REG §3.3/§3.4 verdict backend (P3-B6-2C, R20): the
// blind-pair form's data, the one-act verdict, the decline, the §12 alarm
// disposition and the §4.2.1 consent flip.
//
// ── What this file is NOT ───────────────────────────────────────────────────
//
// It is not the protocol. NO REGISTERED NUMBER, LABEL OR THRESHOLD IS RESTATED
// ANYWHERE IN internal/api — not the gate limbs, not the alarm threshold, not
// the §12 disposition vocabulary, not the four verdict values. Every one of them
// lives in internal/benchmark's registered.go as read-only data pinned to the
// registration file itself, and a copy here would be a second place a frozen
// number could drift (§35; BENCH-REG §17). So the transport passes the caller's
// strings THROUGH to the package, which validates them against the registration,
// and serves the package's own readout shapes back verbatim as raw JSON.
//
// internal/api does not import internal/benchmark either (the reverse wall its
// importwall_test asserts): the pending forms, the reveal and every refusal ride
// the narrow BenchmarkSurface seam below, composed at the shell root — the one
// place allowed to see both vocabularies.
//
// ── The two orderings that are the whole point ──────────────────────────────
//
//   - THE GUESS IS STRUCTURAL. §3.3 is frozen: "No verdict without a guess; the
//     guess is never optional." The package makes a guess-less verdict
//     INEXPRESSIBLE (Answer's fields are unexported and its only constructor
//     demands the guess), so this transport does not validate the guess — it
//     hands both values to the constructor and reports its refusal. A caller
//     cannot route around a type that cannot be built.
//   - THE RECORD PRECEDES THE REVEAL. §3.4: arm identity is readable only after
//     the verdict is recorded. The package commits the §14 record and the pair's
//     move to `recorded` in ONE WriteTx and Reveal is a READ of that committed
//     record — so this handler records FIRST and only then reads, and a reveal
//     asked for before a record simply has nothing to return.

// BenchmarkSurface is the S14.7 practice as this transport speaks it: strings in,
// the package's own shapes out as raw JSON. *benchmark.Practice + *benchmark.Store
// are adapted to it at the shell root; nil leaves every benchmark verb answering
// 503.
//
// Every method takes the ACTOR where the act has an actor, because the package's
// own refusals are data-layer refusals (the §12 operator check and the §4.2.1
// self-only consent rule read the users row) and this transport's scope check is
// an ADDITIONAL gate, never the only one.
type BenchmarkSurface interface {
	// PendingVerdicts returns the caller's pending blind-pair forms as the
	// package renders them — a shape that carries no arm, no position, no run and
	// no model, because the struct has no such field and its query selects no
	// such column (BENCH-REG §3.4). An empty requester means every pair, which is
	// the operator's read, exactly as the landed inbox projection scopes it.
	PendingVerdicts(ctx context.Context, requester string) (json.RawMessage, error)
	// RecordVerdict is the §3.3 one act: the blind pick AND the arm-guess. It
	// returns nothing but its error — the reveal is a separate READ, and keeping
	// them separate is what makes the §3.4 order visible here.
	RecordVerdict(ctx context.Context, pairID, verdict, guess string) error
	// Reveal reads the COMMITTED §14 record, which is the only place arm identity
	// is readable.
	Reveal(ctx context.Context, pairID string) (json.RawMessage, error)
	// Decline is the §4.2.5 veto: still a §14 record, with the decline flag and
	// no verdict, no guess and no artifacts — selective participation must be
	// visible, not silent.
	Decline(ctx context.Context, pairID string) error
	// DisposeAlarm is the §12 operator disposition. The disposition string is the
	// caller's, validated against the registration inside the package.
	DisposeAlarm(ctx context.Context, actor, domain, disposition, reason string) error
	// SetOptIn is the §4.2.1 standing consent flip. It is NOT delegable and the
	// package refuses any actor but the subject themselves — the operator
	// included.
	SetOptIn(ctx context.Context, actor, subject string, enabled bool) error
}

// ── GET /api/benchmark/verdicts — the blind form's data (R20) ───────────────

// BenchmarkVerdictForms is the pending-verdict response. Pairs is the package's
// own pre-record shape, passed through: this transport never re-shapes it,
// because the blindness guarantee is a property of that struct and its query.
type BenchmarkVerdictForms struct {
	Pairs json.RawMessage `json:"pairs"`
	// GuessRequired is stated so a form cannot render without it (BENCH-REG
	// §3.3, frozen). It is a contract fact, not a registered number.
	GuessRequired bool   `json:"guess_required"`
	Detail        string `json:"detail"`
}

func (s *Server) handleBenchmarkVerdicts(w http.ResponseWriter, r *http.Request) {
	if !s.benchmarkReady(w) {
		return
	}
	scope := s.readScope(r)
	// Scope mirrors the landed inbox derivation exactly: the REQUESTER is the
	// voter, so a member reads their own; the operator reads all, as everywhere
	// (S01.9). Visibility is not authorship — the answer path below refuses the
	// operator on somebody else's pair, which is the same line part A drew for
	// asks (D10).
	requester := scope.UserID
	if scope.Operator {
		requester = ""
	}
	pairs, err := s.benchmark.PendingVerdicts(r.Context(), requester)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// UNWRAPPED, and the reason is stronger here than anywhere else on the read
	// surface. The two rendered bodies are the accepted deliverable and the direct
	// arm's own answer — S13/S06 product CONTENT, structurally exempt from the
	// observability primitive (§7-C2·2, §38 ruling (b)) — and they come off the
	// pair row, not out of a run_events payload. Beyond the standing rule, running
	// them through a redactor would put a HEURISTIC between the two arms: a false
	// positive that fired on one side and not the other would change what the
	// voter compared, and this is the one surface where an unequal edit to one arm
	// is a measurement artifact rather than a cosmetic one (BENCH-REG §3.2 — the
	// template is uniform and the platform never edits an arm).
	s.writeReadJSON(w, BenchmarkVerdictForms{
		Pairs: pairs, GuessRequired: true,
		Detail: "each pair is two responses through ONE uniform template, keyed by side. Which arm is which is exactly what the vote must not know, and the verdict form takes the blind pick and the arm-guess in the same act (BENCH-REG §3.2/§3.3)",
	})
}

// ── POST /api/approvals/{id}/verdict — the §3.3 one act (R20) ───────────────

type verdictBody struct {
	// Verdict and Guess are passed through to the package's constructor
	// UNVALIDATED here: the vocabulary is registered text and restating it in
	// this file is exactly the drift §17 forbids.
	Verdict string `json:"verdict"`
	Guess   string `json:"guess"`
}

// VerdictRecorded is the response. Reveal is the COMMITTED §14 record — read
// after the record committed, never a flag flipped on a mutable row.
type VerdictRecorded struct {
	PairID   string          `json:"pair_id"`
	Recorded bool            `json:"recorded"`
	Reveal   json.RawMessage `json:"reveal,omitempty"`
	Detail   string          `json:"detail"`
}

func (s *Server) handleBenchmarkVerdict(w http.ResponseWriter, r *http.Request) {
	pairID, scope, ok := s.benchmarkPairAct(w, r)
	if !ok {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body verdictBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	// THE RECORD FIRST. A refusal here — a missing guess above all — fires
	// nothing: the pair keeps its state and no record exists to reveal.
	if err := s.benchmark.RecordVerdict(r.Context(), pairID, body.Verdict, body.Guess); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// THE REVEAL SECOND, and only as a read of what just committed. A failure to
	// read it back is not a failure of the vote: the record is the artifact, and
	// the response says so rather than implying the verdict did not land.
	reveal, err := s.benchmark.Reveal(r.Context(), pairID)
	if err != nil {
		s.logger.Error("benchmark: verdict recorded but the reveal read failed", "pair", pairID, "err", err)
		s.writeReadRedacted(w, VerdictRecorded{PairID: pairID, Recorded: true,
			Detail: "your verdict is recorded; the arm identities could not be read back just now and are readable from the committed record at any time (BENCH-REG §3.4)"})
		return
	}
	s.logger.Info("benchmark: blind-pair verdict recorded", "pair", pairID, "voter", scope.UserID)
	s.writeReadRedacted(w, VerdictRecorded{PairID: pairID, Recorded: true, Reveal: reveal,
		Detail: "recorded, then revealed: the §14 record commits with the pair's move to recorded in one transaction, and the reveal is a read of that committed record (BENCH-REG §3.4)"})
}

// ── POST /api/approvals/{id}/decline — the §4.2.5 veto (R20) ────────────────

// VerdictDeclined is the response. A decline still writes its §14 record, so
// there is nothing to reveal and the shape has no field for one.
type VerdictDeclined struct {
	PairID   string `json:"pair_id"`
	Declined bool   `json:"declined"`
	Detail   string `json:"detail"`
}

func (s *Server) handleBenchmarkDecline(w http.ResponseWriter, r *http.Request) {
	pairID, scope, ok := s.benchmarkPairAct(w, r)
	if !ok {
		return
	}
	if err := s.benchmark.Decline(r.Context(), pairID); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.logger.Info("benchmark: blind pair declined by its requester", "pair", pairID, "requester", scope.UserID)
	s.writeReadJSON(w, VerdictDeclined{PairID: pairID, Declined: true,
		Detail: "declined, and LOGGED: a declined pair still writes its record with no verdict, no guess and no artifacts, and the decline rate is reported beside every win rate — selective participation must be visible, not silent (BENCH-REG §4.2.5)"})
}

// benchmarkPairAct resolves and authorizes a pair-scoped act, or writes the
// refusal and reports false.
//
// The authority rule is REQUESTER-ONLY and the operator is inside that refusal,
// not outside it: the requester is the voter (BENCH-REG §2/§3.3) — they asked for
// the work, they read both answers, and the vote is their judgement of their own
// deliverable. An administrator voting on somebody else's pair would put a
// judgement in the record under a name that never saw the task. The role bit
// decides VISIBILITY (the operator reads the pending list), never authorship —
// exactly the line part A drew for asks (D10).
func (s *Server) benchmarkPairAct(w http.ResponseWriter, r *http.Request) (string, ownerScope, bool) {
	if !s.projReady(w) || !s.benchmarkReady(w) {
		return "", ownerScope{}, false
	}
	kind, pairID, cut := strings.Cut(r.PathValue("id"), ":")
	if !cut || kind != approvalKindVerdict || pairID == "" {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"this verb takes a blind-pair card id (%s:<pair id>); other kinds have their own verbs", approvalKindVerdict)))
		return "", ownerScope{}, false
	}
	scope := s.readScope(r)
	var owner string
	err := s.proj.db.QueryRowContext(r.Context(),
		`SELECT user_id FROM benchmark_pairs WHERE pair_id = ?`, pairID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: "no such blind pair"})
		return "", ownerScope{}, false
	}
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read blind-pair owner: %w", err))
		return "", ownerScope{}, false
	}
	if !mayAnswerVerdict(scope, owner) {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			Msg: "a blind-pair verdict is its REQUESTER's own judgement and is not delegable — the operator can see the card and cannot vote it (BENCH-REG §2/§3.3)"})
		return "", ownerScope{}, false
	}
	return pairID, scope, true
}

// ── POST /api/approvals/{id}/dispose — the §12 alarm disposition (R20) ──────

type disposeBody struct {
	// Disposition is the caller's, passed through to the package, which checks it
	// against the registration. The vocabulary is registered text (§12) and this
	// file must not carry a copy of it.
	Disposition string `json:"disposition"`
	Reason      string `json:"reason,omitempty"`
}

// AlarmDispositioned is the response: identifiers and the act, no card content.
type AlarmDispositioned struct {
	CardID      string `json:"card_id"`
	Domain      string `json:"domain"`
	Disposition string `json:"disposition"`
	Cleared     bool   `json:"cleared"`
	Detail      string `json:"detail"`
}

func (s *Server) handleBenchmarkAlarmDispose(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.benchmarkReady(w) {
		return
	}
	kind, native, cut := strings.Cut(r.PathValue("id"), ":")
	domain, _, _ := strings.Cut(native, "#")
	if !cut || kind != approvalKindAlarm || domain == "" {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"dispose takes a benchmark alarm card id (%s:<domain>#<epoch>); other kinds have their own verbs", approvalKindAlarm)))
		return
	}
	scope := s.readScope(r)
	// Visibility IS the authority check here, the part-B pattern: alarms are
	// platform-scope and the landed derivation returns none to a member, so asking
	// it answers both "may this caller act?" and "is there an alarm to act on?".
	// The package re-checks the role bit in the data layer regardless — this
	// transport is an additional gate, never the only one (§12).
	alarms, err := s.proj.benchmarkAlarms(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	standing := false
	for _, a := range alarms {
		if a.Domain == domain {
			standing = true
			break
		}
	}
	if !standing {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: "no standing benchmark alarm for that domain is visible to you"})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body disposeBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	id, _ := IdentityFrom(r.Context())
	if err := s.benchmark.DisposeAlarm(r.Context(), id.UserID, domain, body.Disposition, body.Reason); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// NO decision.recorded here (OQ8): DisposeAlarm mints the family-5 row itself,
	// inside the same transaction as the alarm clear. A second row would make one
	// disposition look like two.
	s.writeReadJSON(w, AlarmDispositioned{
		CardID: approvalKindAlarm + ":" + native, Domain: domain, Disposition: body.Disposition, Cleared: true,
		Detail: "dispositioned and logged as a human decision, with the clear kept as alarm history. Nothing was killed, parked or resumed: the expansion freeze this lifts was a visible state, never an enforcement (BENCH-REG §12)",
	})
}

// ── POST /api/benchmark/opt-in — the §4.2.1 standing consent (R20; OQ10) ────

type optInBody struct {
	// Person is optional and defaults to the caller. It exists so a delegation
	// ATTEMPT is expressible and therefore refusable: consent that cannot be
	// asked for on somebody else's behalf cannot be seen to be refused.
	Person  string `json:"person,omitempty"`
	Enabled bool   `json:"enabled"`
}

// OptInSet is the response.
type OptInSet struct {
	Person  string `json:"person"`
	Enabled bool   `json:"enabled"`
	Detail  string `json:"detail"`
}

func (s *Server) handleBenchmarkOptIn(w http.ResponseWriter, r *http.Request) {
	if !s.benchmarkReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body optInBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	id, _ := IdentityFrom(r.Context())
	subject := strings.TrimSpace(body.Person)
	if subject == "" {
		subject = id.UserID
	}
	// SELF-ONLY, and the operator is refused by the same sentence as anyone else
	// (OQ10; §35 "consent is not delegable"). A benchmark whose participation an
	// administrator can switch on for other people is not measuring what §4.2.5
	// exists to protect, and the requester's own automation budget pays for the
	// duplicate run. The package refuses this too — the refusal is stated twice on
	// purpose, and neither statement is the only one.
	if subject != id.UserID {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			Msg: "the benchmark opt-in is the requester's own consent and is not delegable — nobody sets it for anybody else, the operator included (BENCH-REG §4.2.1)"})
		return
	}
	if err := s.benchmark.SetOptIn(r.Context(), id.UserID, subject, body.Enabled); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	// NO decision.recorded here (OQ8): SetOptIn mints the family-5 consent row
	// itself, in the same transaction as the flip.
	s.writeReadJSON(w, OptInSet{Person: subject, Enabled: body.Enabled,
		Detail: "your standing benchmark opt-in is recorded as a human decision with its actor and its timestamp. It defaults OFF and no boot step, migration or administrator turns it on (BENCH-REG §4.2.1)"})
}

// benchmarkReady reports the benchmark seam, or writes the not-wired answer.
func (s *Server) benchmarkReady(w http.ResponseWriter) bool {
	if s.benchmark == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S14.7 benchmark practice is not wired in this process"})
		return false
	}
	return true
}

// mayAnswerVerdict: the pair's REQUESTER, and nobody else. It sits beside the
// other mayAnswer* predicates for the same reason they sit together — the inbox
// card's `answerable` and the verb's own door read the SAME rule, so a surface
// never renders a control whose use would be refused (part A, drain D9).
func mayAnswerVerdict(scope ownerScope, owner string) bool { return scope.UserID == owner }

// mayDisposeAlarm: the §12 disposition is an operator act. The alarm exists
// because the platform may be losing its own benchmark, so letting any principal
// clear it would make the one signal designed to be hard to ignore trivially
// ignorable.
func mayDisposeAlarm(scope ownerScope) bool { return scope.Operator }
