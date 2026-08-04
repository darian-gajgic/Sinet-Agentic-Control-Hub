package auth

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// newStore builds a Store over a real migrated platform.db in t.TempDir(),
// with a controllable clock.
func newStore(t *testing.T) (*Store, *clock) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(db, eventlog.New(db, reg))
	c := &clock{t: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)}
	s.now = c.now
	return s, c
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

const (
	opPIN     = "246810"
	memberPIN = "1357"
)

// seedOperator creates the bootstrap operator "op".
func seedOperator(t *testing.T, s *Store) {
	t.Helper()
	err := s.CreateUser(context.Background(), "", User{ID: "op", DisplayName: "Operator", Role: RoleOperator}, opPIN)
	if err != nil {
		t.Fatalf("bootstrap operator: %v", err)
	}
}

func seedMember(t *testing.T, s *Store) {
	t.Helper()
	err := s.CreateUser(context.Background(), "op", User{ID: "mem", DisplayName: "Member", Role: RoleMember}, memberPIN)
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
}

// ── argon2id / PHC ──

func TestPHCRoundTripAndShape(t *testing.T) {
	phc, err := hashPIN("2468")
	if err != nil {
		t.Fatalf("hashPIN: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("PHC shape = %q, want argon2id with the RFC 9106 low-memory parameters", phc)
	}
	if ok, err := verifyPINHash("2468", phc); err != nil || !ok {
		t.Fatalf("verify correct pin = %v, %v", ok, err)
	}
	if ok, err := verifyPINHash("8642", phc); err != nil || ok {
		t.Fatalf("verify wrong pin = %v, %v; want false", ok, err)
	}
	// A second hash of the same PIN must differ (fresh salt).
	phc2, err := hashPIN("2468")
	if err != nil {
		t.Fatalf("hashPIN: %v", err)
	}
	if phc == phc2 {
		t.Fatal("two hashes of the same PIN are identical — salt not fresh")
	}
}

// TestPHCGoldenVector pins the exact derivation (fixed salt) so a driver or
// parameter regression cannot pass silently — the behavioral conformance
// check of the golang.org/x/crypto adoption (S16.4 #5).
func TestPHCGoldenVector(t *testing.T) {
	salt := []byte("0123456789abcdef") // 16 bytes, fixed
	got := encodePHC("2468", salt, argonTime, argonMemoryKiB, argonThreads)
	// Generated at adoption (x/crypto v0.54.0, 2026-07-19); upstream's own
	// test suite carries the RFC 9106 known-answer vectors — this value
	// guards against silent derivation drift on any future bump.
	const want = "$argon2id$v=19$m=65536,t=3,p=4$MDEyMzQ1Njc4OWFiY2RlZg$gdUfYVgm+D8PozGuF5M7c8Vb4eWDh9UsALgxUqzFycE"
	if got != want {
		t.Fatalf("golden argon2id vector drifted:\n got %s\nwant %s", got, want)
	}
}

func TestPHCVerifyReadsParamsFromString(t *testing.T) {
	// A credential written under different (smaller) parameters must still
	// verify: parameters ride the string, not the code constants.
	salt := []byte("fedcba9876543210")
	phc := encodePHC("2468", salt, 1, 8*1024, 1)
	if ok, err := verifyPINHash("2468", phc); err != nil || !ok {
		t.Fatalf("foreign-params verify = %v, %v", ok, err)
	}
}

func TestPHCMalformedRejected(t *testing.T) {
	for _, phc := range []string{
		"",
		"plainly-not-phc",
		"$argon2i$v=19$m=65536,t=3,p=4$AAAA$BBBB",      // wrong algorithm
		"$argon2id$v=18$m=65536,t=3,p=4$AAAA$BBBB",     // wrong version
		"$argon2id$v=19$m=99999999,t=3,p=4$AAAA$BBBB",  // memory beyond ceiling
		"$argon2id$v=19$m=65536,t=3,p=4$!notb64!$BBBB", // bad salt encoding
		"$argon2id$v=19$m=65536,t=3,p=4$AAAA$",         // empty key
	} {
		if _, err := verifyPINHash("2468", phc); !errors.Is(err, ErrBadPINHash) {
			t.Errorf("verifyPINHash(%q) err = %v, want ErrBadPINHash", phc, err)
		}
	}
}

// ── users ──

func TestBootstrapWindowAndRoles(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()

	// Bootstrap must create an operator.
	err := s.CreateUser(ctx, "", User{ID: "m", Role: RoleMember}, "1234")
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("bootstrap member err = %v, want ErrInvalidRole", err)
	}
	seedOperator(t, s)

	// Window closed: anonymous create now requires a session.
	err = s.CreateUser(ctx, "", User{ID: "x", Role: RoleOperator}, "1234")
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("post-bootstrap anonymous create err = %v, want ErrNoSession", err)
	}

	seedMember(t, s)
	// Members hold no D10 powers.
	err = s.CreateUser(ctx, "mem", User{ID: "x", Role: RoleMember}, "1234")
	if !errors.Is(err, ErrNotOperator) {
		t.Fatalf("member create err = %v, want ErrNotOperator", err)
	}
	// Duplicate id.
	err = s.CreateUser(ctx, "op", User{ID: "mem", Role: RoleMember}, "1234")
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate err = %v, want ErrUserExists", err)
	}
	// Reserved and invalid ids.
	for id, want := range map[string]error{
		"platform":              ErrReservedUserID,
		"dev":                   ErrReservedUserID,
		"":                      ErrInvalidUserID,
		"has space":             ErrInvalidUserID,
		strings.Repeat("x", 65): ErrInvalidUserID,
	} {
		if err := s.CreateUser(ctx, "op", User{ID: id, Role: RoleMember}, "1234"); !errors.Is(err, want) {
			t.Errorf("CreateUser(%q) err = %v, want %v", id, err, want)
		}
	}
	// PIN bounds.
	if err := s.CreateUser(ctx, "op", User{ID: "y", Role: RoleMember}, "123"); !errors.Is(err, ErrInvalidPIN) {
		t.Errorf("short pin err = %v, want ErrInvalidPIN", err)
	}

	users, err := s.Users(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("Users = %v, %v; want op+mem", users, err)
	}
	if !users[0].PINSet || users[0].ID != "mem" || users[1].ID != "op" || users[1].Role != RoleOperator {
		t.Fatalf("Users = %+v", users)
	}
}

// ── login, sessions, expiry ──

func TestLoginSessionLifecycle(t *testing.T) {
	s, c := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)

	if _, err := s.Login(ctx, "op", "wrong-pin", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong pin err = %v", err)
	}
	if _, err := s.Login(ctx, "ghost", opPIN, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user err = %v (must collapse to invalid credentials)", err)
	}

	sess, err := s.Login(ctx, "op", opPIN, "laptop@example")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.UserID != "op" || sess.Token == "" || !sess.Expires.Equal(c.t.Add(SessionTTL)) {
		t.Fatalf("session = %+v", sess)
	}
	if uid, err := s.Validate(ctx, sess.Token); err != nil || uid != "op" {
		t.Fatalf("Validate = %q, %v", uid, err)
	}
	if _, err := s.Validate(ctx, "forged-token"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("forged token err = %v", err)
	}

	// Absolute wall-clock expiry: past the TTL the session is gone and the
	// row is deleted on sight.
	c.advance(SessionTTL + time.Minute)
	if _, err := s.Validate(ctx, sess.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session err = %v", err)
	}
	if _, err := s.Validate(ctx, sess.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session (second read) err = %v", err)
	}

	// Fresh login, then logout revokes.
	sess2, err := s.Login(ctx, "op", opPIN, "")
	if err != nil {
		t.Fatalf("re-login: %v", err)
	}
	if err := s.Logout(ctx, sess2.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := s.Validate(ctx, sess2.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("post-logout err = %v", err)
	}
	if err := s.Logout(ctx, sess2.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("double logout err = %v", err)
	}
}

// ── attempt limiting & lockout ──

func TestPINAttemptLimitAndLockout(t *testing.T) {
	s, c := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)

	// A success resets the counter: 3 failures, a success, then 4 more
	// failures must not lock (counter restarted after the success).
	for i := 0; i < 3; i++ {
		if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d err = %v", i, err)
		}
	}
	if _, err := s.Login(ctx, "op", opPIN, ""); err != nil {
		t.Fatalf("success after failures: %v", err)
	}
	for i := 0; i < maxPINAttempts-1; i++ {
		if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("post-reset failure %d err = %v, want invalid credentials (not locked)", i, err)
		}
	}
	// The ceiling-th consecutive failure engages the lockout.
	if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("ceiling failure err = %v, want ErrPINLocked", err)
	}
	// While locked, even the correct PIN is refused (and not verified).
	if _, err := s.Login(ctx, "op", opPIN, ""); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("locked correct-pin err = %v, want ErrPINLocked", err)
	}
	// Lock expiry: the correct PIN works again and counters clear.
	c.advance(pinLockout + time.Second)
	if _, err := s.Login(ctx, "op", opPIN, ""); err != nil {
		t.Fatalf("post-lockout login: %v", err)
	}
}

func TestLockExpiryClearsCounter(t *testing.T) {
	s, c := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)
	for i := 0; i < maxPINAttempts; i++ {
		s.Login(ctx, "op", "bad", "")
	}
	if _, err := s.Login(ctx, "op", opPIN, ""); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("expected locked, got %v", err)
	}
	c.advance(pinLockout + time.Second)
	// First attempt after expiry is a FAILURE — it must count as attempt 1,
	// not instantly re-lock on the stale counter.
	if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("post-expiry failure err = %v, want plain invalid credentials", err)
	}
	if _, err := s.Login(ctx, "op", opPIN, ""); err != nil {
		t.Fatalf("post-expiry correct login: %v", err)
	}
}

func TestVerifyPINSharesLockoutCounter(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)

	// Failures split across login and step-up re-prompt feed one counter.
	for i := 0; i < 3; i++ {
		if err := s.VerifyPIN(ctx, "op", "bad", "step-up"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("reprompt failure %d err = %v", i, err)
		}
	}
	if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login failure err = %v", err)
	}
	if _, err := s.Login(ctx, "op", "bad", ""); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("5th mixed failure err = %v, want ErrPINLocked", err)
	}
	if err := s.VerifyPIN(ctx, "op", opPIN, "step-up"); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("locked reprompt err = %v, want ErrPINLocked", err)
	}
}

// ── PIN set / reset ──

func TestSetPINRulesAndSessionRevocation(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)
	seedMember(t, s)

	memA, err := s.Login(ctx, "mem", memberPIN, "")
	if err != nil {
		t.Fatalf("mem login: %v", err)
	}
	memB, err := s.Login(ctx, "mem", memberPIN, "")
	if err != nil {
		t.Fatalf("mem login 2: %v", err)
	}

	// Member cannot reset someone else's PIN.
	if err := s.SetPIN(ctx, "mem", "op", "9999", ""); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("member cross-set err = %v, want ErrNotOperator", err)
	}

	// Self-change keeps the acting session, revokes the rest.
	if err := s.SetPIN(ctx, "mem", "mem", "8888", memA.Token); err != nil {
		t.Fatalf("self set: %v", err)
	}
	if _, err := s.Validate(ctx, memA.Token); err != nil {
		t.Fatalf("kept session invalidated: %v", err)
	}
	if _, err := s.Validate(ctx, memB.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("other session survived pin change: %v", err)
	}
	if _, err := s.Login(ctx, "mem", "8888", ""); err != nil {
		t.Fatalf("login with new pin: %v", err)
	}

	// Operator reset clears a lockout and revokes every target session.
	for i := 0; i < maxPINAttempts; i++ {
		s.Login(ctx, "mem", "bad", "")
	}
	if _, err := s.Login(ctx, "mem", "8888", ""); !errors.Is(err, ErrPINLocked) {
		t.Fatalf("expected locked member, got %v", err)
	}
	if err := s.SetPIN(ctx, "op", "mem", "7777", ""); err != nil {
		t.Fatalf("operator reset: %v", err)
	}
	if _, err := s.Login(ctx, "mem", "7777", ""); err != nil {
		t.Fatalf("login after operator reset: %v", err)
	}
	// Unknown target.
	if err := s.SetPIN(ctx, "op", "ghost", "7777", ""); !errors.Is(err, ErrUnknownUser) {
		t.Fatalf("unknown target err = %v", err)
	}
}

// ── device grants ──

func TestGrantLifecycle(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)
	seedMember(t, s)
	const device = "tablet@example"

	// No grant: auto-login refused, logged.
	if _, err := s.GrantLogin(ctx, "mem", device); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("ungranted auto-login err = %v", err)
	}
	// Member cannot grant.
	if err := s.CreateGrant(ctx, "mem", device, "mem"); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("member grant err = %v", err)
	}
	if err := s.CreateGrant(ctx, "op", device, "mem"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// One grant per device: re-pointing requires an explicit revoke.
	if err := s.CreateGrant(ctx, "op", device, "op"); !errors.Is(err, ErrGrantExists) {
		t.Fatalf("double grant err = %v", err)
	}
	// Prefill resolution.
	if uid, ok, err := s.GrantFor(ctx, device); err != nil || !ok || uid != "mem" {
		t.Fatalf("GrantFor = %q, %v, %v", uid, ok, err)
	}
	// Auto-login for the granted user only.
	if _, err := s.GrantLogin(ctx, "op", device); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("mismatched auto-login err = %v", err)
	}
	sess, err := s.GrantLogin(ctx, "mem", device)
	if err != nil || sess.UserID != "mem" {
		t.Fatalf("auto-login = %+v, %v", sess, err)
	}
	// Grant login must not touch the PIN failure counter.
	s.Login(ctx, "mem", "bad", "")
	if _, err := s.GrantLogin(ctx, "mem", device); err != nil {
		t.Fatalf("auto-login after a pin failure: %v", err)
	}

	// Listing is operator-only.
	if _, err := s.Grants(ctx, "mem"); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("member list err = %v", err)
	}
	grants, err := s.Grants(ctx, "op")
	if err != nil || len(grants) != 1 || grants[0] != (Grant{DeviceLogin: device, UserID: "mem", GrantedBy: "op"}) {
		t.Fatalf("Grants = %+v, %v", grants, err)
	}

	// Revoke restores the shared-device default.
	if err := s.RevokeGrant(ctx, "op", device); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.GrantLogin(ctx, "mem", device); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("post-revoke auto-login err = %v", err)
	}
	if err := s.RevokeGrant(ctx, "op", device); !errors.Is(err, ErrNoGrant) {
		t.Fatalf("double revoke err = %v", err)
	}
}

// ── event-log audit trail ──

// TestAuthEventsAttribution asserts Spec S01.9's audit duty: every auth act
// lands on the event log, owner-attributed per 15.6.
func TestAuthEventsAttribution(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)

	s.Login(ctx, "ghost", "1234", "somewhere@dev")     // unknown user → platform-attributed failure
	sess, err := s.Login(ctx, "op", opPIN, "laptop@x") // success
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	s.VerifyPIN(ctx, "op", opPIN, "grant")  // re-prompt
	s.CreateGrant(ctx, "op", "pad@x", "op") // grant
	s.Logout(ctx, sess.Token)

	events, err := s.log.After(ctx, 0, 100)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	type row struct{ typ, user string }
	var got []row
	payloads := map[string]json.RawMessage{}
	for _, e := range events {
		got = append(got, row{e.Type, e.UserID})
		payloads[e.Type] = e.Payload
	}
	want := []row{
		{EventUserCreated, "op"},
		{EventLoginFailed, "platform"}, // unknown user: no owning row exists
		{EventLogin, "op"},
		{EventReprompt, "op"},
		{EventGrant, "op"},
		{EventLogout, "op"},
	}
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Payload spot-checks: the platform-attributed failure names the
	// attempted user; the login carries its via + device hint.
	var fail struct {
		AttemptedUser string `json:"attempted_user"`
		Reason        string `json:"reason"`
	}
	if err := json.Unmarshal(payloads[EventLoginFailed], &fail); err != nil ||
		fail.AttemptedUser != "ghost" || fail.Reason != "unknown_user" {
		t.Fatalf("login_failed payload = %s (%v)", payloads[EventLoginFailed], err)
	}
	var login struct {
		Via         string `json:"via"`
		DeviceLogin string `json:"device_login"`
	}
	if err := json.Unmarshal(payloads[EventLogin], &login); err != nil ||
		login.Via != "pin" || login.DeviceLogin != "laptop@x" {
		t.Fatalf("login payload = %s (%v)", payloads[EventLogin], err)
	}
	var reprompt struct {
		Purpose string `json:"purpose"`
	}
	if err := json.Unmarshal(payloads[EventReprompt], &reprompt); err != nil || reprompt.Purpose != "grant" {
		t.Fatalf("reprompt payload = %s (%v)", payloads[EventReprompt], err)
	}
}

// TestLockoutEventPayload asserts the failure event carries the lock
// signal when the ceiling engages.
func TestLockoutEventPayload(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	seedOperator(t, s)
	for i := 0; i < maxPINAttempts; i++ {
		s.Login(ctx, "op", "bad", "")
	}
	events, err := s.log.After(ctx, 0, 100)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	last := events[len(events)-1]
	if last.Type != EventLoginFailed {
		t.Fatalf("last event = %s", last.Type)
	}
	var p struct {
		Locked   bool `json:"locked"`
		Attempts int  `json:"attempts"`
	}
	if err := json.Unmarshal(last.Payload, &p); err != nil || !p.Locked || p.Attempts != maxPINAttempts {
		t.Fatalf("lockout payload = %s (%v)", last.Payload, err)
	}
}
