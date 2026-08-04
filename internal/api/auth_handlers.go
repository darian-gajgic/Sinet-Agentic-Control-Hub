package api

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

// The login/session endpoints of Spec S01.9 (S15.2: "Login/session
// endpoints are S01.9's"). Exact paths and schemas are P3 work within the
// S15.2 bounds. The pre-session surface — session state, the user picker,
// login — is reachable without a session because layer 1 already holds:
// reaching a login screen proves tailnet membership (Spec S01.9).
//
// High-tier semantics (S15.6, G3 Def.1) at B0: every credential- or
// grant-changing verb re-prompts the acting user's own PIN in the same
// request (VerifyPIN before the act) — approval identity is never inherited
// from an idle session. When the approvals family lands (B2+), its High-
// tier cards route through the same VerifyPIN machinery.

// maxAuthBodyBytes bounds an auth request body.
const maxAuthBodyBytes = 1 << 16

// ── request/response shapes ──

type loginRequest struct {
	UserID string `json:"user_id"`
	// PIN empty = grant auto-login attempt (Spec S01.9 layer 2).
	PIN string `json:"pin"`
}

type loginResponse struct {
	UserID  string    `json:"user_id"`
	Expires time.Time `json:"expires"`
}

type sessionResponse struct {
	Authenticated bool       `json:"authenticated"`
	Dev           bool       `json:"dev,omitempty"`
	User          *auth.User `json:"user,omitempty"`
	Hint          *hintInfo  `json:"hint,omitempty"`
}

type hintInfo struct {
	DeviceLogin string `json:"device_login"`
	// UserID/AutoLogin report an operator grant mapping this device to a
	// user ("it prefills the login picker … it may complete login").
	UserID    string `json:"user_id,omitempty"`
	AutoLogin bool   `json:"auto_login,omitempty"`
}

type verifyPINRequest struct {
	PIN string `json:"pin"`
}

type setPINRequest struct {
	// UserID empty = self. Operator-only otherwise (household PIN reset).
	UserID string `json:"user_id"`
	// PIN is the acting user's current PIN — the step-up re-prompt.
	PIN    string `json:"pin"`
	NewPIN string `json:"new_pin"`
}

type createUserRequest struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	// PIN is the new user's initial PIN (mandatory — a PIN-less row cannot
	// log in).
	PIN string `json:"pin"`
	// ActorPIN is the acting operator's step-up re-prompt; absent in the
	// first-boot bootstrap window.
	ActorPIN string `json:"actor_pin"`
}

type grantRequest struct {
	DeviceLogin string `json:"device_login"`
	UserID      string `json:"user_id,omitempty"`
	// PIN is the acting operator's step-up re-prompt.
	PIN string `json:"pin"`
}

// ── handlers ──

// handleAuthSession reports the caller's identity state plus the device
// hint — the login-picker prefill contract (Spec S01.9 layer 2).
func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	resp := sessionResponse{}
	if id, ok := IdentityFrom(r.Context()); ok {
		resp.Authenticated = true
		resp.Dev = id.Dev
		if !id.Dev {
			u, err := s.sessions.User(r.Context(), id.UserID)
			if err != nil {
				s.authError(w, err)
				return
			}
			resp.User = &u
		}
	}
	if hint := DeviceHint(r); hint != "" {
		h := hintInfo{DeviceLogin: hint}
		if userID, ok, err := s.sessions.GrantFor(r.Context(), hint); err != nil {
			s.authError(w, err)
			return
		} else if ok {
			h.UserID, h.AutoLogin = userID, true
		}
		resp.Hint = &h
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// handleAuthUsers serves the pre-session user picker (Spec S01.9: "user
// picker + per-user PIN"; reaching it already proves tailnet membership).
func (s *Server) handleAuthUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.sessions.Users(r.Context())
	if err != nil {
		s.authError(w, err)
		return
	}
	if users == nil {
		users = []auth.User{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// handleAuthLogin mints a session: PIN login, or grant auto-login when the
// request carries no PIN (Spec S01.9 layers 2–3).
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	hint := DeviceHint(r)
	var (
		sess auth.Session
		err  error
	)
	if req.PIN == "" {
		sess, err = s.sessions.GrantLogin(r.Context(), req.UserID, hint)
		if errors.Is(err, auth.ErrNoGrant) {
			http.Error(w, "no device grant for auto-login", http.StatusUnauthorized)
			return
		}
	} else {
		sess, err = s.sessions.Login(r.Context(), req.UserID, req.PIN, hint)
	}
	if err != nil {
		s.authError(w, err)
		return
	}
	s.setSessionCookie(w, sess)
	s.writeJSON(w, http.StatusOK, loginResponse{UserID: sess.UserID, Expires: sess.Expires})
}

// handleAuthLogout revokes the current session and clears the cookie.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		if err := s.sessions.Logout(r.Context(), c.Value); err != nil && !errors.Is(err, auth.ErrNoSession) {
			s.authError(w, err)
			return
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthVerifyPIN is the naked step-up verb (Spec S01.9/S15.6 High-tier
// re-prompt): verify the acting user's own PIN now, regardless of session
// age.
func (s *Server) handleAuthVerifyPIN(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	var req verifyPINRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if err := s.sessions.VerifyPIN(r.Context(), id.UserID, req.PIN, "step-up"); err != nil {
		s.authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthSetPIN sets or resets a PIN: self-change, or operator reset of
// another user. The acting user's current PIN is re-prompted first.
func (s *Server) handleAuthSetPIN(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	var req setPINRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	target := req.UserID
	if target == "" {
		target = id.UserID
	}
	if err := s.sessions.VerifyPIN(r.Context(), id.UserID, req.PIN, "pin_set"); err != nil {
		s.authError(w, err)
		return
	}
	keep := ""
	if target == id.UserID {
		if c, err := r.Cookie(SessionCookieName); err == nil {
			keep = c.Value
		}
	}
	if err := s.sessions.SetPIN(r.Context(), id.UserID, target, req.NewPIN, keep); err != nil {
		s.authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthUserCreate creates a person row. Operator-only with a step-up
// re-prompt; the one exception is the first-boot bootstrap window (users
// table empty — the store enforces it), where the request may be anonymous
// and must create the operator.
func (s *Server) handleAuthUserCreate(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	actor := ""
	if id, ok := IdentityFrom(r.Context()); ok && !id.Dev {
		// The dev fallback identity is resolution-only: it carries no
		// authority, so it takes the anonymous path (bootstrap window only).
		actor = id.UserID
		if err := s.sessions.VerifyPIN(r.Context(), actor, req.ActorPIN, "user_create"); err != nil {
			s.authError(w, err)
			return
		}
	}
	u := auth.User{ID: req.UserID, DisplayName: req.DisplayName, Role: req.Role}
	if err := s.sessions.CreateUser(r.Context(), actor, u, req.PIN); err != nil {
		s.authError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"user_id": req.UserID})
}

// handleAuthGrants lists device grants (operator-only).
func (s *Server) handleAuthGrants(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	if s.rejectDev(w, id) {
		return
	}
	grants, err := s.sessions.Grants(r.Context(), id.UserID)
	if err != nil {
		s.authError(w, err)
		return
	}
	if grants == nil {
		grants = []auth.Grant{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"grants": grants})
}

// handleAuthGrantCreate records a trusted auto-login grant (Spec S01.9
// layer 2; a permission change — operator + step-up re-prompt).
func (s *Server) handleAuthGrantCreate(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	if s.rejectDev(w, id) {
		return
	}
	var req grantRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.DeviceLogin == "" || req.UserID == "" {
		http.Error(w, "device_login and user_id required", http.StatusBadRequest)
		return
	}
	if err := s.sessions.VerifyPIN(r.Context(), id.UserID, req.PIN, "grant"); err != nil {
		s.authError(w, err)
		return
	}
	if err := s.sessions.CreateGrant(r.Context(), id.UserID, req.DeviceLogin, req.UserID); err != nil {
		if errors.Is(err, auth.ErrNoGrant) { // unusable device_login value
			http.Error(w, "invalid device_login", http.StatusBadRequest)
			return
		}
		s.authError(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, map[string]string{"device_login": req.DeviceLogin, "user_id": req.UserID})
}

// handleAuthGrantRevoke removes a grant — the device reverts to the
// shared-device default (PIN always required, G3 Def.1).
func (s *Server) handleAuthGrantRevoke(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	if s.rejectDev(w, id) {
		return
	}
	var req grantRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}
	if req.DeviceLogin == "" {
		http.Error(w, "device_login required", http.StatusBadRequest)
		return
	}
	if err := s.sessions.VerifyPIN(r.Context(), id.UserID, req.PIN, "grant_revoke"); err != nil {
		s.authError(w, err)
		return
	}
	if err := s.sessions.RevokeGrant(r.Context(), id.UserID, req.DeviceLogin); err != nil {
		if errors.Is(err, auth.ErrNoGrant) {
			http.Error(w, "no such grant", http.StatusNotFound)
			return
		}
		s.authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── plumbing ──

// rejectDev blocks the dev fallback identity from authority-bearing verbs:
// it is resolution-only (P3/CONVENTIONS.md §7); admin acts require a real
// session even in dev posture.
func (s *Server) rejectDev(w http.ResponseWriter, id Identity) bool {
	if id.Dev {
		http.Error(w, "dev fallback identity carries no authority; log in", http.StatusForbidden)
		return true
	}
	return false
}

// decodeJSON enforces the JSON content type (a cheap cross-site hardening
// on top of SameSite=Lax), bounds the body, and decodes strictly.
func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return false
	}
	return true
}

// writeJSON writes one JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Warn("api: encode response", "err", err)
	}
}

// authError maps store errors onto the HTTP surface. Login-shaped failures
// stay collapsed (401) — the event log carries the precise reason, the
// response does not.
func (s *Server) authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrNoSession):
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	case errors.Is(err, auth.ErrPINLocked):
		http.Error(w, "PIN locked after repeated failures; retry later", http.StatusTooManyRequests)
	case errors.Is(err, auth.ErrNotOperator):
		http.Error(w, "operator role required", http.StatusForbidden)
	case errors.Is(err, auth.ErrUserExists), errors.Is(err, auth.ErrGrantExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, auth.ErrUnknownUser):
		http.Error(w, "unknown user", http.StatusNotFound)
	case errors.Is(err, auth.ErrInvalidPIN), errors.Is(err, auth.ErrInvalidUserID),
		errors.Is(err, auth.ErrInvalidRole), errors.Is(err, auth.ErrReservedUserID):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		s.logger.Error("api: auth", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// setSessionCookie hands the bearer token to the browser. SameSite=Lax so
// push-notification deep links (top-level GET navigations, Spec S15.11)
// carry the session; Secure except in dev posture (dev serves plain HTTP on
// loopback; in production TLS terminates at tailscale serve, Spec S01.4).
func (s *Server) setSessionCookie(w http.ResponseWriter, sess auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sess.Token,
		Path:     "/",
		MaxAge:   int(time.Until(sess.Expires).Seconds()),
		HttpOnly: true,
		Secure:   !s.devPosture,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !s.devPosture,
		SameSite: http.SameSiteLaxMode,
	})
}
