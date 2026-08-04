// Package push is the S15.11 Web Push channel: the durable subscription
// registry, the RFC 8291/8292 sender, and the VAPID signing key the platform
// identifies itself to browser push services with.
//
// It is a LEAF over storage + eventlog + the standard library, held directly by
// the composition root the way internal/chat and internal/review are (§39/§44).
// It performs no evaluation and derives no dueness: WHICH decisions are waiting
// for whom is the per-identity derivation that already lives in internal/api,
// and re-deriving it here would be the twin-maintained-copy hazard (§40-C D3).
// This package answers three narrower questions — where may I reach this
// person, how do I seal a payload to that device, and when did each of their
// cards last go out.
//
// WHAT LEAVES THIS MACHINE, exactly. Row 2 of the S01.8 accepted-external-
// observables register: "Push timing/volume → Browser-vendor push services
// (Apple/Google relays) → Timing, volume, endpoint metadata [leave] → Payload
// content (encrypted to subscription keys) [never leaves]". Every outbound
// request this package makes goes to a STORED SUBSCRIPTION ENDPOINT and to
// nothing else — there is no other absolute URL in the package, no probe, no
// analytics, no asset fetch — and the body is sealed to that subscription's own
// keys before it is written. The register is spec-owned and this package does
// not widen it.
//
// THE ENDPOINT IS A CAPABILITY. Anyone holding one can push to that device, so
// it is stored, used and NEVER served: every read here answers with
// EndpointHash, and every event payload carries the hash too. That is the whole
// reason the column exists (migration 0021).
package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The three event types this family mints (B6-9 OQ9). All three ADMIT into
// family 12 (Platform) under the same broadened reading that homes worker.*,
// registry.*, preview.*, local.* and chat.* (§29 OQ2-(A)): a subscription is an
// owner-attributed control-plane asset and a send is the platform auditing its
// own outward act. None of them is a RUN event — a push has no run, no
// objective and no receipt. No new family, so no S00.9 amendment.
//
// FAILURE IS AN OUTCOME FIELD ON push.sent, NOT A SECOND TYPE. One send is one
// act and it lands one row; a `push.send_failed` sibling would double-mint the
// same act's shape and force every reader to union two types to answer "when
// did this last go out" — which is exactly the question the dueness derivation
// asks.
const (
	EventSubscribed   = "push.subscribed"
	EventUnsubscribed = "push.unsubscribed"
	EventSent         = "push.sent"
)

// The recorded outcomes of one send attempt, as the audit row's `outcome`
// field. A closed vocabulary, because a reader branches on it.
const (
	// OutcomeSent — the push service accepted the message for delivery. It is
	// NOT a delivery receipt: no such thing exists in RFC 8030.
	OutcomeSent = "sent"
	// OutcomeGone — 404/410. The subscription is dead and has been removed.
	OutcomeGone = "gone"
	// OutcomeRefused — the push service answered with any other error status.
	OutcomeRefused = "refused"
	// OutcomeUnreachable — the request never got an answer.
	OutcomeUnreachable = "unreachable"
)

// The recorded reasons a subscription left, as the `push.unsubscribed`
// payload's `reason` field.
//
// A CLOSED VOCABULARY, and the two values are two different facts: a person
// turning notifications off on their own device, and the platform retiring an
// endpoint the push service reported gone. The first is deliberately NOT called
// "operator" — `operator` is the D10 ROLE word everywhere else in this
// codebase, and a member unenrolling their own phone would have recorded a row
// naming a role they do not hold, for somebody auditing who did what to read.
const (
	ReasonOwnerUnenrolled = "owner_unenrolled"
	ReasonPushServiceGone = "push_service_gone"
)

// ErrNotFound is "no such subscription for this caller". It deliberately covers
// "not yours" as well as "not there": an id that answers differently for a
// stranger is an existence oracle for somebody else's device (the internal/chat
// precedent, §44).
var ErrNotFound = errors.New("push: subscription not found")

// ErrBadEnrolment marks an enrolment this platform will not store. Every case
// is a boundary check on a body a browser composed, which is the only place
// this package validates anything.
var ErrBadEnrolment = errors.New("push: unusable enrolment")

// Bounds. Structural constants with named reasons — S18 ratifies no push key
// and the index is byte-frozen, so these are code (the §7 sseBatchSize / §9
// auth-constant precedent, interim under the standing settings-tab directive).
const (
	// MaxSubscriptionsPerUser bounds one person's device list. A household
	// member has a phone, a tablet and a laptop; a hundred rows for one identity
	// means a client is looping, and an unbounded list is an unbounded fan-out
	// on every evaluation. The refusal names the bound rather than silently
	// dropping the oldest — a device that thinks it is enrolled and is not is
	// the worst failure this family has.
	MaxSubscriptionsPerUser = 20
	// MaxEndpointBytes bounds the stored endpoint. Real endpoints run to a few
	// hundred characters; this is a sanity ceiling on a value that becomes a
	// request target, not a protocol limit.
	MaxEndpointBytes = 2048
	// MaxLabelRunes bounds the person's own device label. It is a label, not a
	// note.
	MaxLabelRunes = 64
	// SendTimeout bounds one outbound request to a push service. Long enough
	// for a relay on a slow link, short enough that one unreachable service
	// cannot hold an evaluation pass open behind it.
	SendTimeout = 20 * time.Second
)

// Subscription is one stored row, as the platform holds it. The Endpoint field
// is the capability and is never serialized outward — Metadata below is what
// every read surface answers with.
type Subscription struct {
	ID           string
	UserID       string
	Endpoint     string
	EndpointHash string
	Keys         Keys
	Origin       string
	DeviceLogin  string
	Label        string
	CreatedTS    time.Time
	UpdatedTS    time.Time
}

// Metadata is one subscription as anyone may READ it: the hash that names it,
// the device context, and the enrolling origin. There is no endpoint here and
// none may be added (B6-9 OQ2 — the operator sees rows as metadata).
type Metadata struct {
	ID           string `json:"id"`
	Owner        string `json:"owner"`
	EndpointHash string `json:"endpoint_hash"`
	Origin       string `json:"origin"`
	DeviceLogin  string `json:"device_login,omitempty"`
	Label        string `json:"label,omitempty"`
	CreatedTS    string `json:"created_ts"`
	UpdatedTS    string `json:"updated_ts"`
}

// Metadata renders the row for a read surface.
func (s Subscription) Metadata() Metadata {
	return Metadata{
		ID: s.ID, Owner: s.UserID, EndpointHash: s.EndpointHash, Origin: s.Origin,
		DeviceLogin: s.DeviceLogin, Label: s.Label,
		CreatedTS: s.CreatedTS.UTC().Format(time.RFC3339),
		UpdatedTS: s.UpdatedTS.UTC().Format(time.RFC3339),
	}
}

// Config assembles a Store.
type Config struct {
	DB  *storage.DB
	Log *eventlog.Log
	// StateDir is where the VAPID signing key lives (B6-9 OQ8(b)): the control
	// plane holds it at v0, 0600 under the systemd StateDirectory, and moving
	// custody to the broker is a named hardening-session item.
	StateDir string
	// Now is the clock seam, for the same reason review.Store.Now and
	// chat.Config.Now exist: a golden fixture produced through the real verbs
	// cannot carry a wall-clock stamp. nil = time.Now.
	Now func() time.Time
	// NewID mints subscription ids; nil = a random hex id. Same seam, same
	// reason (§45-B), and no caller can choose an id in production.
	NewID  func() string
	Logger *slog.Logger
}

// Store is the subscription registry and the send path over it.
type Store struct {
	db       *storage.DB
	log      *eventlog.Log
	stateDir string
	now      func() time.Time
	newID    func() string
	logger   *slog.Logger

	vapid *vapidKey
}

// New opens the store and loads (or mints) the VAPID signing key.
func New(cfg Config) (*Store, error) {
	if cfg.DB == nil || cfg.Log == nil {
		return nil, errors.New("push: DB and Log are required")
	}
	if cfg.StateDir == "" {
		return nil, errors.New("push: StateDir is required: the VAPID key has to live somewhere durable")
	}
	s := &Store{db: cfg.DB, log: cfg.Log, stateDir: cfg.StateDir, now: cfg.Now, newID: cfg.NewID, logger: cfg.Logger}
	if s.now == nil {
		s.now = time.Now
	}
	if s.newID == nil {
		s.newID = randomID
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	key, err := loadOrCreateVAPIDKey(cfg.StateDir)
	if err != nil {
		return nil, err
	}
	s.vapid = key
	return s, nil
}

// PublicKey is the base64url `applicationServerKey` a browser must subscribe
// with. It is served to the enrolment surface rather than compiled into the
// SPA, because the key is per-installation and a client constant would be a
// second copy of it that silently stops matching after a rotation.
func (s *Store) PublicKey() string { return s.vapid.publicKey }

// signingKey is the ECDSA half, used only by the sender.
func (s *Store) signingKey() *ecdsa.PrivateKey { return s.vapid.key }

// Enrol stores or replaces one subscription for the authenticated identity.
//
// It is RETRY-SAFE by construction: the row is keyed (user, endpoint), so a
// browser handing back the endpoint it already has updates that row rather than
// making a second device out of it, and the answer says which happened.
func (s *Store) Enrol(ctx context.Context, userID string, in Enrolment) (Subscription, bool, error) {
	if userID == "" {
		return Subscription{}, false, fmt.Errorf("%w: no identity", ErrBadEnrolment)
	}
	sub, err := in.validate(userID)
	if err != nil {
		return Subscription{}, false, err
	}
	now := s.now().UTC()
	sub.CreatedTS, sub.UpdatedTS = now, now
	sub.ID = s.newID()

	var replaced bool
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var existingID, createdTS string
		row := tx.QueryRowContext(ctx,
			`SELECT subscription_id, created_ts FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`,
			userID, sub.Endpoint)
		switch err := row.Scan(&existingID, &createdTS); {
		case err == nil:
			replaced = true
			sub.ID = existingID
			sub.CreatedTS = parseTS(createdTS)
			if _, err := tx.ExecContext(ctx,
				`UPDATE push_subscriptions SET p256dh = ?, auth = ?, origin = ?, device_login = ?, label = ?, updated_ts = ?
				  WHERE subscription_id = ?`,
				sub.Keys.P256DH, sub.Keys.Auth, sub.Origin, sub.DeviceLogin, sub.Label,
				now.Format(time.RFC3339Nano), existingID); err != nil {
				return fmt.Errorf("push: replace subscription: %w", err)
			}
		case errors.Is(err, sql.ErrNoRows):
			var n int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, userID).Scan(&n); err != nil {
				return fmt.Errorf("push: count subscriptions: %w", err)
			}
			if n >= MaxSubscriptionsPerUser {
				return fmt.Errorf("%w: %d devices already enrolled, the bound is %d", ErrBadEnrolment, n, MaxSubscriptionsPerUser)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO push_subscriptions
				   (subscription_id, user_id, endpoint, endpoint_hash, p256dh, auth, origin, device_login, label, created_ts, updated_ts)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				sub.ID, userID, sub.Endpoint, sub.EndpointHash, sub.Keys.P256DH, sub.Keys.Auth,
				sub.Origin, sub.DeviceLogin, sub.Label,
				now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("push: insert subscription: %w", err)
			}
		default:
			return fmt.Errorf("push: read subscription: %w", err)
		}
		return s.appendTx(ctx, tx, userID, EventSubscribed, subscriptionEvent{
			Subscription: sub.ID, EndpointHash: sub.EndpointHash, Origin: sub.Origin,
			Replaced: replaced,
		}, now)
	})
	if err != nil {
		return Subscription{}, false, err
	}
	sub.UserID = userID
	return sub, replaced, nil
}

// Remove deletes one subscription of the caller's, by endpoint. It is
// idempotent: removing a subscription that is already gone answers
// applied:false rather than an error, which is what makes a phone retry safe.
func (s *Store) Remove(ctx context.Context, userID, endpoint string) (bool, error) {
	if userID == "" || endpoint == "" {
		return false, fmt.Errorf("%w: identity and endpoint are required", ErrBadEnrolment)
	}
	return s.removeWhere(ctx, userID, `user_id = ? AND endpoint = ?`, []any{userID, endpoint}, ReasonOwnerUnenrolled)
}

// removeDead drops a subscription the push service reported gone (404/410).
// The reason rides the audit row so the record distinguishes a person's own act
// from the platform retiring an endpoint that stopped existing.
func (s *Store) removeDead(ctx context.Context, sub Subscription) error {
	_, err := s.removeWhere(ctx, sub.UserID, `subscription_id = ?`, []any{sub.ID}, ReasonPushServiceGone)
	return err
}

func (s *Store) removeWhere(ctx context.Context, userID, where string, args []any, reason string) (bool, error) {
	var applied bool
	now := s.now().UTC()
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var id, hash string
		row := tx.QueryRowContext(ctx,
			`SELECT subscription_id, endpoint_hash FROM push_subscriptions WHERE `+where, args...)
		if err := row.Scan(&id, &hash); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("push: read subscription: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE subscription_id = ?`, id); err != nil {
			return fmt.Errorf("push: delete subscription: %w", err)
		}
		applied = true
		return s.appendTx(ctx, tx, userID, EventUnsubscribed, subscriptionEvent{
			Subscription: id, EndpointHash: hash, Reason: reason,
		}, now)
	})
	return applied, err
}

// List answers one identity's own subscriptions, oldest first.
func (s *Store) List(ctx context.Context, userID string) ([]Subscription, error) {
	if userID == "" {
		return nil, fmt.Errorf("%w: no identity", ErrBadEnrolment)
	}
	return s.query(ctx, `WHERE user_id = ? ORDER BY created_ts, subscription_id`, userID)
}

// ListAll answers every subscription in the household, oldest first. It is the
// OPERATOR's read (B6-9 OQ2, the runs/telemetry posture of §30): a subscription
// is platform machinery — which devices the notifier may reach — and not
// personal content, so the operator sees the rows. What they see is Metadata,
// exactly like every other caller: nobody is served an endpoint.
func (s *Store) ListAll(ctx context.Context) ([]Subscription, error) {
	return s.query(ctx, `ORDER BY user_id, created_ts, subscription_id`)
}

// Owners answers the identities with at least one subscription — the notifier's
// candidate set, so an evaluation costs nothing for a household that has not
// enrolled a device.
func (s *Store) Owners(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT user_id FROM push_subscriptions ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("push: owners: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("push: owner scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) query(ctx context.Context, tail string, args ...any) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT subscription_id, user_id, endpoint, endpoint_hash, p256dh, auth, origin, device_login, label, created_ts, updated_ts
		   FROM push_subscriptions `+tail, args...)
	if err != nil {
		return nil, fmt.Errorf("push: list subscriptions: %w", err)
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		var s Subscription
		var created, updated string
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.EndpointHash, &s.Keys.P256DH, &s.Keys.Auth,
			&s.Origin, &s.DeviceLogin, &s.Label, &created, &updated); err != nil {
			return nil, fmt.Errorf("push: subscription scan: %w", err)
		}
		s.CreatedTS, s.UpdatedTS = parseTS(created), parseTS(updated)
		out = append(out, s)
	}
	// The ITERATION error lives only in Err(): a scan that failed mid-way would
	// otherwise serve a SHORT list as a complete one (§45-B).
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("push: subscription rows: %w", err)
	}
	return out, nil
}

// Enrolment is what a browser hands over after PushManager.subscribe.
type Enrolment struct {
	Endpoint    string
	Keys        Keys
	Origin      string
	DeviceLogin string
	Label       string
}

// validate is the boundary check on a browser-composed body. Every refusal here
// is about a value that would otherwise become a request target or a deep-link
// prefix, which is why the checks are here and not spread across callers.
func (e Enrolment) validate(userID string) (Subscription, error) {
	endpoint := strings.TrimSpace(e.Endpoint)
	if endpoint == "" {
		return Subscription{}, fmt.Errorf("%w: endpoint is empty", ErrBadEnrolment)
	}
	if len(endpoint) > MaxEndpointBytes {
		return Subscription{}, fmt.Errorf("%w: endpoint is %d bytes, the bound is %d", ErrBadEnrolment, len(endpoint), MaxEndpointBytes)
	}
	// An endpoint becomes an outbound request, so it is checked as one: https
	// with a host, and nothing else. vapidAudience is the same parse the sender
	// performs, so a row that stores can always be signed for.
	if _, err := vapidAudience(endpoint); err != nil {
		return Subscription{}, fmt.Errorf("%w: %w", ErrBadEnrolment, err)
	}
	// The key material is checked HERE rather than at first send, so a device
	// that cannot be reached is refused while somebody is looking at the screen
	// instead of failing silently a day later.
	if _, _, err := e.Keys.decode(); err != nil {
		return Subscription{}, fmt.Errorf("%w: %w", ErrBadEnrolment, err)
	}
	// The origin becomes the prefix of every deep link sent to this device, so
	// it is checked with the same parse and stored as an ORIGIN, never a URL
	// with a path.
	origin, err := normalizeOrigin(e.Origin)
	if err != nil {
		return Subscription{}, fmt.Errorf("%w: %w", ErrBadEnrolment, err)
	}
	label := strings.TrimSpace(e.Label)
	if len([]rune(label)) > MaxLabelRunes {
		return Subscription{}, fmt.Errorf("%w: label is longer than %d characters", ErrBadEnrolment, MaxLabelRunes)
	}
	return Subscription{
		UserID:       userID,
		Endpoint:     endpoint,
		EndpointHash: HashEndpoint(endpoint),
		Keys:         e.Keys,
		Origin:       origin,
		DeviceLogin:  strings.TrimSpace(e.DeviceLogin),
		Label:        label,
	}, nil
}

// HashEndpoint is the one derivation of a subscription's public REFERENCE.
// Every surface and every event payload names a subscription by this and never
// by the endpoint itself.
func HashEndpoint(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

// subscriptionEvent is the contract-minimum payload of the two lifecycle types:
// refs, a closed vocabulary and no content. There is no endpoint here.
type subscriptionEvent struct {
	Subscription string `json:"subscription"`
	EndpointHash string `json:"endpoint_hash"`
	Origin       string `json:"origin,omitempty"`
	Replaced     bool   `json:"replaced,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func (s *Store) appendTx(ctx context.Context, tx *sql.Tx, userID, typ string, payload any, now time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("push: marshal %s: %w", typ, err)
	}
	if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
		UserID: userID, Type: typ, SchemaVersion: 1, Payload: raw, Time: now,
	}); err != nil {
		return fmt.Errorf("push: append %s: %w", typ, err)
	}
	return nil
}

func parseTS(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
