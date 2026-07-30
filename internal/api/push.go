package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
)

// push.go is the transport of the S15.11 Web Push family: which of a person's
// devices the notifier may reach.
//
// A NEW RESOURCE ROOT, REPORTED RATHER THAN ASSUMED (B6-9 OQ2). The S15.2
// family table has no push row — S15.11 names the channel and the payload
// contract and no root, no owner column and no storage — so the family lands
// additive-first under `/api/push`, under S15.2's own contract RULES
// (owner-scoped server-side, every mutation on the event log, retry-safe, no
// outward-effect verb), exactly as `/api/chat` did at B6-7. A cosmetic table
// amendment naming the row is carried to the B6 gate list, not taken here — the
// executor never edits Spec/.
//
// THE ENDPOINT NEVER COMES BACK OUT. Every read here answers with
// push.Metadata: the endpoint hash, the device label, the enrolling origin. An
// endpoint is a capability URL, so serving one — even to its own owner, even to
// the operator — would hand a reader the ability to push to that device.
//
// NO PIN, AND THE REASON IS THE TIER RULE. Enrolling a device releases nothing
// outward and the pushes it enables carry no decision content: they carry a
// title, a deep link and a count, all composed by the platform. That is
// squarely reversible-inside-the-workspace (S15.6 Low), so there is no step-up
// here and no batch verb either.

// pushEnrolBody is the enrol/replace request. The origin is the ENROLLING
// PAGE's own (B6-9 OQ7): `navigate` must be absolute and the control plane
// binds loopback behind the S01.4 front chain, so it cannot see the origin a
// person reaches it by — but the page can, and it is that device's own deep-link
// prefix. It is format-checked server-side rather than trusted.
type pushEnrolBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Origin string `json:"origin"`
	Label  string `json:"label"`
}

// PushSubscriptionsView is the one read this family serves.
type PushSubscriptionsView struct {
	// VAPIDPublicKey is the `applicationServerKey` a browser must subscribe
	// with. It is SERVED rather than compiled into the SPA because the key is
	// per-installation: a client constant would be a second copy that silently
	// stops matching the day the key is rotated.
	VAPIDPublicKey string `json:"vapid_public_key"`
	// Subscriptions is what this caller may read: their own devices, or — for
	// the operator — every device in the household, as metadata (OQ2).
	Subscriptions []push.Metadata `json:"subscriptions"`
	// Scope is the reading this body IS, as a sentence. A narrower reading must
	// never be able to pose as the wider one (the §45-B roster_scope precedent).
	Scope string `json:"scope"`
	// Contact is the RFC 8292 posture, stated so the enrolment surface can say
	// what leaves this machine without the client inventing the sentence.
	Observable string `json:"observable"`
	Cursor     int64  `json:"cursor"`
}

// The two scope sentences.
const (
	pushScopeOperator = "every device enrolled in this household, as metadata: an endpoint is a capability and is never served"
	pushScopeOwn      = "your own enrolled devices"
)

// pushObservable is the S01.8 register row this family lives under, served as
// text so the enrolment surface states it from data rather than from a sentence
// somebody wrote into a component.
const pushObservable = "Push timing and volume reach your browser vendor's push service (Apple or Google). The notification itself is encrypted to this device's own keys, so the relay sees when and how often — never what."

func (s *Server) pushReady(w http.ResponseWriter) bool {
	if s.push == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the push channel is not wired in this process"})
		return false
	}
	return true
}

// GET /api/push/subscriptions
func (s *Server) handlePushList(w http.ResponseWriter, r *http.Request) {
	if !s.pushReady(w) || !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	var subs []push.Subscription
	view := PushSubscriptionsView{VAPIDPublicKey: s.push.PublicKey(), Observable: pushObservable, Cursor: cursor}
	if scope.Operator {
		subs, err = s.push.ListAll(r.Context())
		view.Scope = pushScopeOperator
	} else {
		subs, err = s.push.List(r.Context(), scope.UserID)
		view.Scope = pushScopeOwn
	}
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	view.Subscriptions = []push.Metadata{}
	for _, sub := range subs {
		view.Subscriptions = append(view.Subscriptions, sub.Metadata())
	}
	s.writeReadJSON(w, view)
}

// POST /api/push/subscriptions — enrol or replace this device.
func (s *Server) handlePushEnrol(w http.ResponseWriter, r *http.Request) {
	if !s.pushReady(w) {
		return
	}
	var body pushEnrolBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: "body is not JSON"})
		return
	}
	id, _ := IdentityFrom(r.Context()) // present: the route is requireIdentity
	sub, replaced, err := s.push.Enrol(r.Context(), id.UserID, push.Enrolment{
		Endpoint: body.Endpoint,
		Keys:     push.Keys{P256DH: body.Keys.P256DH, Auth: body.Keys.Auth},
		Origin:   body.Origin,
		// The S01.9 layer-2 hint, recorded as AUDIT CONTEXT and never as
		// authority: the authenticated identity is what scopes the row.
		DeviceLogin: DeviceHint(r),
		Label:       body.Label,
	})
	if err != nil {
		s.writePushErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Subscription push.Metadata `json:"subscription"`
		// Replaced is the retry-safe answer: a browser re-posting the endpoint
		// it already holds gets the already-resolved state rather than a second
		// device (S15.2).
		Replaced bool `json:"replaced"`
	}{sub.Metadata(), replaced})
}

// POST /api/push/subscriptions/remove — this device is leaving.
//
// It takes the ENDPOINT rather than the subscription id because that is what a
// browser has: `pushManager.getSubscription()` answers with the endpoint, and
// the id is the platform's own. A body naming somebody else's endpoint simply
// matches nothing (the read is keyed on the caller), so there is no cross-owner
// case to refuse separately.
func (s *Server) handlePushRemove(w http.ResponseWriter, r *http.Request) {
	if !s.pushReady(w) {
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: "body is not JSON"})
		return
	}
	id, _ := IdentityFrom(r.Context())
	applied, err := s.push.Remove(r.Context(), id.UserID, body.Endpoint)
	if err != nil {
		s.writePushErr(w, err)
		return
	}
	// applied:false is the honest repeat — the device was already gone and this
	// request changed nothing, which is what makes a phone retry safe (S15.2).
	s.writeReadJSON(w, struct {
		Applied bool `json:"applied"`
	}{applied})
}

// writePushErr maps the store's errors onto statuses by TYPE, never by message
// text (§38).
func (s *Server) writePushErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, push.ErrBadEnrolment), errors.Is(err, push.ErrBadSubscription):
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()})
	case errors.Is(err, push.ErrNotFound):
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()})
	default:
		s.logger.Error("push surface", "err", err)
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()})
	}
}
