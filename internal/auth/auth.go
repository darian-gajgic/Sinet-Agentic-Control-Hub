// Package auth is layer 3 of the Spec S01.9 authentication stack — the
// authoritative person identity: server-side sessions as owner-attributed
// rows in platform.db, per-user PIN credentials (argon2id), and the
// per-device trusted auto-login grants of layer 2's operator-grant clause.
// Layers 1 and 2 are not code here: the tailnet wall is posture (loopback
// listeners behind the S01.4 front chain, linted fail-closed, P-T13-2) and
// the device hint is a request header parsed at the API seam.
//
// AuthZ is enforced in this data layer per Spec S01.9: accessors take the
// acting identity and check the one role bit (operator vs member, D10)
// against the users row — handlers never carry authority of their own.
// Every auth event — login, failure, grant, re-prompt — lands on the event
// log in the same transaction as its row change (Spec S01.9, S01.11).
package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Session and PIN policy constants. Spec S01.9 ratifies the mechanisms —
// sessions, PIN, event-logged failures — but names no numbers, and Spec S18
// ratifies no auth ⚙ keys, so per the established convention (the
// sseBatchSize precedent, P3/CONVENTIONS.md §7) these are plain code
// constants, never registry settings. Making any of them operator-dialable
// requires an S00.9 amendment plus the S18 sweep; the set is flagged in the
// B0 gate report.
const (
	// SessionTTL is the absolute wall-clock lifetime of a session row.
	// Expiry is absolute, not sliding: High-tier acts re-prompt the PIN
	// regardless (Spec S01.9), so a long-lived read session is in-model and
	// keeps the notified→glance→decide<10s phone loop (S15.11) working.
	SessionTTL = 30 * 24 * time.Hour
	// maxPINAttempts is the consecutive-failure ceiling before lockout.
	maxPINAttempts = 5
	// pinLockout is how long a locked PIN refuses verification.
	pinLockout = 15 * time.Minute
	// minPINLen/maxPINLen bound the PIN/password string (S01.9 names
	// "PIN/password"; the credential is an opaque string).
	minPINLen = 4
	maxPINLen = 128
)

// Roles — the one role bit of Spec S01.9 implementing D10 co-approval.
const (
	RoleOperator = "operator"
	RoleMember   = "member"
)

// platformActor attributes auth events that have no owning person row (an
// unknown-user login attempt). It equals shell.PlatformUserID — restated
// here because the shell sits above this package (same pattern as
// run.ActorPlatform, P3-B0-4).
const platformActor = "platform"

// DevUserID is the dev-posture fallback identity (established at B0-3).
// It is reserved: no person row may take it, so dev-mode artifacts can
// never collide with a real household member.
const DevUserID = "dev"

// Provisional auth event types, pending the S14 event contract (B5) —
// the same provisional-naming posture as the platform lifecycle and run
// event types (P3/CONVENTIONS.md §7–8). The set covers Spec S01.9's
// "every auth event (login, failure, grant, re-prompt)" plus the
// credential/user administration acts implied by the same audit posture.
const (
	EventLogin          = "auth.login"
	EventLoginFailed    = "auth.login_failed"
	EventLogout         = "auth.logout"
	EventPINSet         = "auth.pin_set"
	EventGrant          = "auth.grant"
	EventGrantRevoked   = "auth.grant_revoked"
	EventReprompt       = "auth.reprompt"
	EventRepromptFailed = "auth.reprompt_failed"
	EventUserCreated    = "auth.user_created"
)

// authEventSchemaVersion versions the auth event payloads.
const authEventSchemaVersion = 1

// Errors callers branch on. Login-shaped failures collapse into
// ErrInvalidCredentials deliberately — the event log carries the precise
// reason, the API response does not.
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrPINLocked          = errors.New("auth: PIN locked after repeated failures")
	ErrNoSession          = errors.New("auth: no valid session")
	ErrNotOperator        = errors.New("auth: operator role required (D10)")
	ErrUnknownUser        = errors.New("auth: unknown user")
	ErrUserExists         = errors.New("auth: user already exists")
	ErrReservedUserID     = errors.New("auth: reserved user id")
	ErrInvalidUserID      = errors.New("auth: invalid user id")
	ErrInvalidRole        = errors.New("auth: role must be operator or member")
	ErrInvalidPIN         = fmt.Errorf("auth: PIN must be %d–%d characters", minPINLen, maxPINLen)
	ErrNoGrant            = errors.New("auth: no device grant")
	ErrGrantExists        = errors.New("auth: device grant already exists (revoke first)")
)

// Store is the S01.9 data layer over platform.db. All methods are safe for
// concurrent use; every write commits its event append in the same
// transaction.
type Store struct {
	db  *storage.DB
	log *eventlog.Log
	now func() time.Time // injectable clock (tests)
}

// New returns a Store over the open database and event log.
func New(db *storage.DB, log *eventlog.Log) *Store {
	return &Store{db: db, log: log, now: time.Now}
}

// User is one person row as the auth stack sees it.
type User struct {
	ID          string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	// PINSet reports whether a credential exists; the hash never leaves the
	// data layer.
	PINSet bool `json:"pin_set"`
}

// Grant is one per-device trusted auto-login grant (Spec S01.9 layer 2).
type Grant struct {
	DeviceLogin string `json:"device_login"`
	UserID      string `json:"user_id"`
	GrantedBy   string `json:"granted_by"`
}

// Session is one minted session. Token is the bearer secret handed to the
// client; only its SHA-256 is stored.
type Session struct {
	Token   string
	UserID  string
	Expires time.Time
}

// ── users ──

// CreateUser creates a person row with an initial PIN. actor is the acting
// authenticated user and must hold the operator role (D10); the one
// exception is the first-boot bootstrap window: while the users table is
// empty, actor may be "" and the created user MUST be an operator —
// otherwise the household would begin without anyone holding D10 powers.
// The initial PIN is mandatory: a user without a credential cannot complete
// the S01.9 login, so a PIN-less row would be a dead account. The caller
// (API layer) re-prompts the actor's own PIN first (S15.6 High-tier
// semantics for permission changes).
func (s *Store) CreateUser(ctx context.Context, actor string, u User, pin string) error {
	if err := validateUserID(u.ID); err != nil {
		return err
	}
	if u.Role != RoleOperator && u.Role != RoleMember {
		return ErrInvalidRole
	}
	if err := validatePIN(pin); err != nil {
		return err
	}
	phc, err := hashPIN(pin) // expensive; outside the write tx
	if err != nil {
		return err
	}
	nowTS := s.timestamp()
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			return fmt.Errorf("auth: count users: %w", err)
		}
		eventActor := actor
		switch {
		case actor == "" && count > 0:
			return ErrNoSession
		case actor == "" && u.Role != RoleOperator:
			// Bootstrap window: the first user must be the operator.
			return ErrInvalidRole
		case actor == "":
			eventActor = u.ID // self-created at bootstrap
		default:
			if err := requireOperatorTx(ctx, tx, actor); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, display_name, role, credential_store_ref, created_ts,
			                    pin_hash, pin_failed_attempts, pin_locked_until_ts)
			 VALUES (?, ?, ?, '', ?, ?, 0, NULL)
			 ON CONFLICT (user_id) DO NOTHING`,
			u.ID, u.DisplayName, u.Role, nowTS, phc)
		if err != nil {
			return fmt.Errorf("auth: insert user: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("auth: insert user: %w", err)
		} else if n == 0 {
			return ErrUserExists
		}
		return s.appendTx(ctx, tx, u.ID, EventUserCreated, map[string]any{
			"actor": eventActor,
			"role":  u.Role,
		})
	})
}

// Users lists every person row — the pre-session login picker data (Spec
// S01.9: "user picker"; reaching it already proves tailnet membership).
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, display_name, role, pin_hash <> '' FROM users ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Role, &u.PINSet); err != nil {
			return nil, fmt.Errorf("auth: scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// User returns one person row, or ErrUnknownUser.
func (s *Store) User(ctx context.Context, id string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, display_name, role, pin_hash <> '' FROM users WHERE user_id = ?`, id).
		Scan(&u.ID, &u.DisplayName, &u.Role, &u.PINSet)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("%w: %q", ErrUnknownUser, id)
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: read user: %w", err)
	}
	return u, nil
}

// ── PIN verification core ──

// credential is the verification-relevant slice of a users row.
type credential struct {
	pinHash     string
	attempts    int
	lockedUntil time.Time // zero when not locked
}

// readCredential loads the credential columns, or ErrUnknownUser.
func (s *Store) readCredential(ctx context.Context, userID string) (credential, error) {
	var (
		c      credential
		locked sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT pin_hash, pin_failed_attempts, pin_locked_until_ts FROM users WHERE user_id = ?`,
		userID).Scan(&c.pinHash, &c.attempts, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return credential{}, fmt.Errorf("%w: %q", ErrUnknownUser, userID)
	}
	if err != nil {
		return credential{}, fmt.Errorf("auth: read credential: %w", err)
	}
	if locked.Valid && locked.String != "" {
		t, err := time.Parse(time.RFC3339Nano, locked.String)
		if err != nil {
			return credential{}, fmt.Errorf("auth: parse pin_locked_until_ts: %w", err)
		}
		c.lockedUntil = t
	}
	return c, nil
}

// authenticatePIN is the shared verify core of login and step-up re-prompt:
// unknown-user handling, lockout state, argon2id verification, and failure
// counting. It returns nil when the PIN verified — the caller owns the
// success action (counter reset + success event, atomically with whatever
// else success means) — and ErrInvalidCredentials / ErrPINLocked on failure
// with the failType event appended. extra is merged into event payloads.
func (s *Store) authenticatePIN(ctx context.Context, userID, pin, failType string, extra map[string]any) error {
	cred, err := s.readCredential(ctx, userID)
	if errors.Is(err, ErrUnknownUser) {
		// No owning row exists: attribute to the platform actor with the
		// attempted id in the payload (15.6 needs a non-empty owner).
		p := payload(extra, map[string]any{"attempted_user": userID, "reason": "unknown_user"})
		if aerr := s.append(ctx, platformActor, failType, p); aerr != nil {
			return aerr
		}
		return ErrInvalidCredentials
	}
	if err != nil {
		return err
	}

	now := s.now()
	if !cred.lockedUntil.IsZero() {
		if now.Before(cred.lockedUntil) {
			p := payload(extra, map[string]any{"reason": "locked"})
			if aerr := s.append(ctx, userID, failType, p); aerr != nil {
				return aerr
			}
			return ErrPINLocked
		}
		// Lock expired: clear it (and the counter that fed it) before
		// verifying, so one stale failure cannot instantly re-lock.
		err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`UPDATE users SET pin_failed_attempts = 0, pin_locked_until_ts = NULL WHERE user_id = ?`,
				userID)
			return err
		})
		if err != nil {
			return fmt.Errorf("auth: clear expired lock: %w", err)
		}
	}

	if cred.pinHash == "" {
		p := payload(extra, map[string]any{"reason": "no_pin"})
		if aerr := s.append(ctx, userID, failType, p); aerr != nil {
			return aerr
		}
		return ErrInvalidCredentials
	}

	ok, err := verifyPINHash(pin, cred.pinHash) // expensive; outside any tx
	if err != nil {
		return err
	}
	if !ok {
		return s.recordPINFailure(ctx, userID, failType, extra)
	}
	return nil
}

// resetPINCounterTx clears the consecutive-failure counter and any lockout
// — called only on a successful PIN proof.
func (s *Store) resetPINCounterTx(ctx context.Context, tx *sql.Tx, userID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET pin_failed_attempts = 0, pin_locked_until_ts = NULL WHERE user_id = ?`,
		userID); err != nil {
		return fmt.Errorf("auth: reset attempts: %w", err)
	}
	return nil
}

// recordPINFailure advances the consecutive-failure counter atomically,
// engages the lockout at the ceiling, and appends the failure event — one
// transaction, so the audit row can never detach from the counter move.
func (s *Store) recordPINFailure(ctx context.Context, userID, failType string, extra map[string]any) error {
	locked := false
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET pin_failed_attempts = pin_failed_attempts + 1 WHERE user_id = ?`,
			userID); err != nil {
			return fmt.Errorf("auth: count failure: %w", err)
		}
		var attempts int
		if err := tx.QueryRowContext(ctx,
			`SELECT pin_failed_attempts FROM users WHERE user_id = ?`, userID).Scan(&attempts); err != nil {
			return fmt.Errorf("auth: read attempts: %w", err)
		}
		if attempts >= maxPINAttempts {
			locked = true
			until := s.now().Add(pinLockout).UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx,
				`UPDATE users SET pin_locked_until_ts = ? WHERE user_id = ?`, until, userID); err != nil {
				return fmt.Errorf("auth: engage lockout: %w", err)
			}
		}
		p := payload(extra, map[string]any{"reason": "bad_pin", "attempts": attempts})
		if locked {
			p["locked"] = true
		}
		return s.appendTx(ctx, tx, userID, failType, p)
	})
	if err != nil {
		return err
	}
	if locked {
		return ErrPINLocked
	}
	return ErrInvalidCredentials
}

// VerifyPIN is the step-up re-prompt of Spec S01.9/S15.6: verify the acting
// user's own PIN at the moment of a High-tier act — approval identity is
// never inherited from an idle session. purpose names the act in the event
// payload. Failures share the login lockout counter: it is the same secret
// under the same guessing surface.
func (s *Store) VerifyPIN(ctx context.Context, userID, pin, purpose string) error {
	extra := map[string]any{"purpose": purpose}
	if err := s.authenticatePIN(ctx, userID, pin, EventRepromptFailed, extra); err != nil {
		return err
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := s.resetPINCounterTx(ctx, tx, userID); err != nil {
			return err
		}
		return s.appendTx(ctx, tx, userID, EventReprompt, payload(extra, nil))
	})
}

// SetPIN sets or resets a user's PIN. Rules (S01.9 AuthZ in the data
// layer): a user may change their own PIN; an operator may reset any
// user's. The API layer re-prompts the actor's current PIN first via
// VerifyPIN (S15.6 High-tier semantics). Every session of the target is
// revoked except keepToken (the actor's own session on a self-change);
// the target's lockout state clears — an operator reset is the household
// recovery path for a locked PIN.
func (s *Store) SetPIN(ctx context.Context, actor, target, newPIN, keepToken string) error {
	if err := validatePIN(newPIN); err != nil {
		return err
	}
	if _, err := s.User(ctx, target); err != nil {
		return err
	}
	phc, err := hashPIN(newPIN) // expensive; outside the write tx
	if err != nil {
		return err
	}
	keepID := ""
	if keepToken != "" {
		keepID = tokenID(keepToken)
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if actor != target {
			if err := requireOperatorTx(ctx, tx, actor); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET pin_hash = ?, pin_failed_attempts = 0, pin_locked_until_ts = NULL
			 WHERE user_id = ?`, phc, target); err != nil {
			return fmt.Errorf("auth: set pin: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM sessions WHERE user_id = ? AND session_id <> ?`, target, keepID); err != nil {
			return fmt.Errorf("auth: revoke sessions on pin set: %w", err)
		}
		return s.appendTx(ctx, tx, target, EventPINSet, map[string]any{
			"actor": actor,
			"reset": actor != target,
		})
	})
}

// ── shared helpers ──

// requireOperatorTx asserts the actor exists and holds the operator role —
// the one role bit implementing D10 (Spec S01.9), read from the row inside
// the acting transaction, never trusted from a request.
func requireOperatorTx(ctx context.Context, tx *sql.Tx, actor string) error {
	if actor == "" {
		return ErrNoSession
	}
	var role string
	err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE user_id = ?`, actor).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: acting user %q", ErrUnknownUser, actor)
	}
	if err != nil {
		return fmt.Errorf("auth: read actor role: %w", err)
	}
	if role != RoleOperator {
		return ErrNotOperator
	}
	return nil
}

// append writes one auth event in its own transaction.
func (s *Store) append(ctx context.Context, userID, eventType string, p map[string]any) error {
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return s.appendTx(ctx, tx, userID, eventType, p)
	})
}

// appendTx writes one auth event inside the caller's transaction, owner-
// attributed per 15.6.
func (s *Store) appendTx(ctx context.Context, tx *sql.Tx, userID, eventType string, p map[string]any) error {
	if p == nil {
		p = map[string]any{}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("auth: marshal %s payload: %w", eventType, err)
	}
	_, err = s.log.AppendTx(ctx, tx, eventlog.Append{
		UserID:        userID,
		Type:          eventType,
		SchemaVersion: authEventSchemaVersion,
		Payload:       raw,
		Time:          s.now(),
	})
	if err != nil {
		return fmt.Errorf("auth: append %s: %w", eventType, err)
	}
	return nil
}

// payload merges extra over base into a fresh map.
func payload(extra, base map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// timestamp renders the house wall-clock format (RFC 3339, UTC — recorded,
// never an ordering authority; Spec S02.5, P-T07-4).
func (s *Store) timestamp() string {
	return s.now().UTC().Format(time.RFC3339Nano)
}

// validateUserID bounds person ids: non-empty, ≤64 chars, printable without
// spaces, and never a reserved platform identity.
func validateUserID(id string) error {
	if id == "" || len(id) > 64 {
		return ErrInvalidUserID
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return ErrInvalidUserID
		}
	}
	if id == platformActor || id == DevUserID {
		return fmt.Errorf("%w: %q", ErrReservedUserID, id)
	}
	return nil
}

// validatePIN bounds the credential string.
func validatePIN(pin string) error {
	if len(pin) < minPINLen || len(pin) > maxPINLen || strings.ContainsFunc(pin, unicode.IsControl) {
		return ErrInvalidPIN
	}
	return nil
}
