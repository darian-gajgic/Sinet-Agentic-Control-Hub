package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"unicode"
)

// sessionTokenLen is the bearer-token entropy in bytes.
const sessionTokenLen = 32

// maxDeviceLoginLen bounds the recorded device hint.
const maxDeviceLoginLen = 256

// tokenID derives the stored session id from a bearer token: SHA-256 hex.
// The token itself is never stored, so a read of platform.db never yields a
// usable credential.
func tokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Login is the PIN login of Spec S01.9 layer 3: verify the user's PIN
// (argon2id, attempt-limited) and mint a session. deviceLogin is the
// Tailscale-User-Login hint observed on the request — recorded for audit,
// never authority. The auth.login event commits in the same transaction as
// the session row.
func (s *Store) Login(ctx context.Context, userID, pin, deviceLogin string) (Session, error) {
	deviceLogin = sanitizeDeviceLogin(deviceLogin)
	extra := map[string]any{"via": "pin"}
	if deviceLogin != "" {
		extra["device_login"] = deviceLogin
	}
	if err := s.authenticatePIN(ctx, userID, pin, EventLoginFailed, extra); err != nil {
		return Session{}, err
	}
	// resetPIN: this success is a real PIN proof.
	return s.mintSession(ctx, userID, deviceLogin, extra, true)
}

// GrantLogin is the layer-2 auto-login path: on a personal device with an
// explicit operator grant, the device hint may complete login (Spec S01.9).
// The requested user must match the granted user exactly; anything else is
// a logged failure. The PIN lockout does not apply — no secret is being
// guessed; the trust here is the operator's recorded grant.
func (s *Store) GrantLogin(ctx context.Context, requestedUser, deviceLogin string) (Session, error) {
	deviceLogin = sanitizeDeviceLogin(deviceLogin)
	extra := map[string]any{"via": "grant"}
	if deviceLogin != "" {
		extra["device_login"] = deviceLogin
	}
	fail := func(reason string) (Session, error) {
		owner := platformActor
		if _, err := s.User(ctx, requestedUser); err == nil {
			owner = requestedUser
		}
		p := payload(extra, map[string]any{"attempted_user": requestedUser, "reason": reason})
		if aerr := s.append(ctx, owner, EventLoginFailed, p); aerr != nil {
			return Session{}, aerr
		}
		return Session{}, ErrNoGrant
	}
	if deviceLogin == "" {
		return fail("no_device_hint")
	}
	granted, ok, err := s.GrantFor(ctx, deviceLogin)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return fail("no_grant")
	}
	if granted != requestedUser {
		return fail("grant_mismatch")
	}
	// resetPIN false: a grant login proves the operator's recorded device
	// trust, not the PIN — it never touches the PIN failure counter.
	return s.mintSession(ctx, granted, deviceLogin, extra, false)
}

// mintSession inserts the session row, sweeps expired rows, and appends
// auth.login — one transaction, so a login event always has its session.
// resetPIN additionally clears the PIN failure counter (PIN logins only).
func (s *Store) mintSession(ctx context.Context, userID, deviceLogin string, extra map[string]any, resetPIN bool) (Session, error) {
	raw := make([]byte, sessionTokenLen)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("auth: generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	sess := Session{Token: token, UserID: userID, Expires: now.Add(SessionTTL)}
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if resetPIN {
			if err := s.resetPINCounterTx(ctx, tx, userID); err != nil {
				return err
			}
		}
		if err := s.sweepExpiredTx(ctx, tx, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sessions (session_id, user_id, device_login, created_ts, expires_ts)
			 VALUES (?, ?, ?, ?, ?)`,
			tokenID(token), userID, deviceLogin,
			now.UTC().Format(time.RFC3339Nano),
			sess.Expires.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("auth: insert session: %w", err)
		}
		return s.appendTx(ctx, tx, userID, EventLogin, payload(extra, nil))
	})
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

// Validate resolves a bearer token to its user id. Expiry is absolute
// wall-clock (a suspend never extends a login); an expired row is deleted
// on sight. Returns ErrNoSession for anything but a live session.
func (s *Store) Validate(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrNoSession
	}
	id := tokenID(token)
	var (
		userID    string
		expiresTS string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, expires_ts FROM sessions WHERE session_id = ?`, id).
		Scan(&userID, &expiresTS)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoSession
	}
	if err != nil {
		return "", fmt.Errorf("auth: read session: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresTS)
	if err != nil {
		return "", fmt.Errorf("auth: parse session expiry: %w", err)
	}
	if !s.now().Before(expires) {
		derr := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE session_id = ?`, id)
			return err
		})
		if derr != nil {
			return "", fmt.Errorf("auth: delete expired session: %w", derr)
		}
		return "", ErrNoSession
	}
	return userID, nil
}

// Logout revokes the token's session and appends auth.logout.
func (s *Store) Logout(ctx context.Context, token string) error {
	userID, err := s.Validate(ctx, token)
	if err != nil {
		return err
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM sessions WHERE session_id = ?`, tokenID(token)); err != nil {
			return fmt.Errorf("auth: delete session: %w", err)
		}
		return s.appendTx(ctx, tx, userID, EventLogout, nil)
	})
}

// sweepExpiredTx lazily deletes expired session rows (the table is
// household-sized; login-time sweeping needs no background loop).
// Timestamps are parsed, never string-compared — RFC 3339 nano fractions
// vary in length.
func (s *Store) sweepExpiredTx(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT session_id, expires_ts FROM sessions`)
	if err != nil {
		return fmt.Errorf("auth: sweep sessions: %w", err)
	}
	var expired []string
	for rows.Next() {
		var id, ts string
		if err := rows.Scan(&id, &ts); err != nil {
			rows.Close()
			return fmt.Errorf("auth: sweep scan: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			rows.Close()
			return fmt.Errorf("auth: sweep parse expiry: %w", err)
		}
		if !now.Before(t) {
			expired = append(expired, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("auth: sweep sessions: %w", err)
	}
	rows.Close()
	for _, id := range expired {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE session_id = ?`, id); err != nil {
			return fmt.Errorf("auth: sweep delete: %w", err)
		}
	}
	return nil
}

// ── device grants (Spec S01.9 layer 2) ──

// CreateGrant records the explicit operator grant that marks a device
// personal: the given Tailscale-User-Login may thereafter complete login as
// userID without a PIN. Operator-only (a permission change, D10/S15.6
// High); the API layer re-prompts the operator's PIN first. A device
// account holds at most one grant — re-pointing requires an explicit
// revoke, so the audit trail shows both acts.
func (s *Store) CreateGrant(ctx context.Context, actor, deviceLogin, userID string) error {
	deviceLogin = sanitizeDeviceLogin(deviceLogin)
	if deviceLogin == "" {
		return fmt.Errorf("%w: empty device login", ErrNoGrant)
	}
	if _, err := s.User(ctx, userID); err != nil {
		return err
	}
	nowTS := s.timestamp()
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireOperatorTx(ctx, tx, actor); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO device_grants (device_login, user_id, granted_by, created_ts)
			 VALUES (?, ?, ?, ?) ON CONFLICT (device_login) DO NOTHING`,
			deviceLogin, userID, actor, nowTS)
		if err != nil {
			return fmt.Errorf("auth: insert grant: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("auth: insert grant: %w", err)
		} else if n == 0 {
			return ErrGrantExists
		}
		return s.appendTx(ctx, tx, userID, EventGrant, map[string]any{
			"actor":        actor,
			"device_login": deviceLogin,
		})
	})
}

// RevokeGrant removes a device grant — the device reverts to the shared-
// device default: PIN always required (G3 Def.1). Operator-only.
func (s *Store) RevokeGrant(ctx context.Context, actor, deviceLogin string) error {
	deviceLogin = sanitizeDeviceLogin(deviceLogin)
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireOperatorTx(ctx, tx, actor); err != nil {
			return err
		}
		var userID string
		err := tx.QueryRowContext(ctx,
			`SELECT user_id FROM device_grants WHERE device_login = ?`, deviceLogin).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", ErrNoGrant, deviceLogin)
		}
		if err != nil {
			return fmt.Errorf("auth: read grant: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM device_grants WHERE device_login = ?`, deviceLogin); err != nil {
			return fmt.Errorf("auth: delete grant: %w", err)
		}
		return s.appendTx(ctx, tx, userID, EventGrantRevoked, map[string]any{
			"actor":        actor,
			"device_login": deviceLogin,
		})
	})
}

// Grants lists every device grant. Operator-only: the grant table is the
// household's device-trust map.
func (s *Store) Grants(ctx context.Context, actor string) ([]Grant, error) {
	var out []Grant
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireOperatorTx(ctx, tx, actor); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT device_login, user_id, granted_by FROM device_grants ORDER BY device_login`)
		if err != nil {
			return fmt.Errorf("auth: list grants: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var g Grant
			if err := rows.Scan(&g.DeviceLogin, &g.UserID, &g.GrantedBy); err != nil {
				return fmt.Errorf("auth: scan grant: %w", err)
			}
			out = append(out, g)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GrantFor resolves a device hint to its granted user, if any — the
// pre-session picker prefill ("suggests the account", Spec S01.9 layer 2).
func (s *Store) GrantFor(ctx context.Context, deviceLogin string) (string, bool, error) {
	deviceLogin = sanitizeDeviceLogin(deviceLogin)
	if deviceLogin == "" {
		return "", false, nil
	}
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM device_grants WHERE device_login = ?`, deviceLogin).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("auth: read grant: %w", err)
	}
	return userID, true, nil
}

// sanitizeDeviceLogin bounds and cleans a device-hint value; anything with
// control characters or beyond the length cap reads as absent — a hint is
// advisory and never worth failing a request over.
func sanitizeDeviceLogin(v string) string {
	if len(v) > maxDeviceLoginLen {
		return ""
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return v
}
