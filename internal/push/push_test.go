package push_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Everything in this file is hermetic: no test dials a push service, a live
// unit, or the network. The one HTTP surface exercised anywhere in the package
// is an httptest server on loopback (send_test.go).

type env struct {
	t        *testing.T
	ctx      context.Context
	db       *storage.DB
	log      *eventlog.Log
	store    *push.Store
	stateDir string
	now      time.Time
	ids      int
}

func newEnv(t *testing.T) *env {
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
	log := eventlog.New(db, reg)
	e := &env{t: t, ctx: ctx, db: db, log: log,
		stateDir: filepath.Join(t.TempDir(), "state"),
		now:      time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)}
	e.store = e.open()
	return e
}

// open builds a store over the SAME database and state directory, so a test can
// throw the process away and prove the answers come from stored state.
func (e *env) open() *push.Store {
	e.t.Helper()
	st, err := push.New(push.Config{
		DB: e.db, Log: e.log, StateDir: e.stateDir,
		Now:   func() time.Time { return e.now },
		NewID: func() string { e.ids++; return "push-fixed-" + string(rune('a'+e.ids-1)) },
	})
	if err != nil {
		e.t.Fatalf("push.New: %v", err)
	}
	return st
}

func (e *env) events(typ string) []map[string]any {
	e.t.Helper()
	rows, err := e.db.QueryContext(e.ctx,
		`SELECT user_id, payload FROM run_events WHERE type = ? ORDER BY event_seq`, typ)
	if err != nil {
		e.t.Fatalf("read %s: %v", typ, err)
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var owner, payload string
		if err := rows.Scan(&owner, &payload); err != nil {
			e.t.Fatalf("scan: %v", err)
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			e.t.Fatalf("payload: %v", err)
		}
		m["_owner"] = owner
		out = append(out, m)
	}
	return out
}

// freshKeys mints a real subscription key pair, so nothing in these tests
// depends on hand-written key material the crypto would reject.
func freshKeys(t *testing.T) push.Keys {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("auth: %v", err)
	}
	b := base64.RawURLEncoding
	return push.Keys{P256DH: b.EncodeToString(priv.PublicKey().Bytes()), Auth: b.EncodeToString(auth)}
}

func enrolment(t *testing.T, endpoint string) push.Enrolment {
	t.Helper()
	return push.Enrolment{
		Endpoint: endpoint, Keys: freshKeys(t),
		Origin: "https://sinet.example.ts.net", DeviceLogin: "alice@example.com", Label: "Alice's phone",
	}
}

// TestEnrolIsRetrySafeAndKeyedPerDevice drives the property that makes a phone
// retry harmless: the row is keyed (person, endpoint), so re-posting the same
// subscription REPLACES rather than making a second device out of it — while a
// genuinely different endpoint is a second device, which is the control that
// stops the assertion passing on a store that simply never inserts.
func TestEnrolIsRetrySafeAndKeyedPerDevice(t *testing.T) {
	e := newEnv(t)
	first, replaced, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/aaa"))
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	if replaced {
		t.Error("a first enrolment reported itself a replacement")
	}

	e.now = e.now.Add(time.Hour)
	again, replaced, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/aaa"))
	if err != nil {
		t.Fatalf("re-enrol: %v", err)
	}
	if !replaced {
		t.Error("re-posting the same endpoint did not report a replacement")
	}
	if again.ID != first.ID {
		t.Errorf("re-enrol minted a new row %s (was %s)", again.ID, first.ID)
	}
	if !again.CreatedTS.Equal(first.CreatedTS) {
		t.Error("a replacement rewrote created_ts")
	}
	if !again.UpdatedTS.After(first.UpdatedTS) {
		t.Error("a replacement did not move updated_ts")
	}

	// The control: a different endpoint IS a second device.
	if _, _, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/bbb")); err != nil {
		t.Fatalf("second device: %v", err)
	}
	subs, err := e.store.List(e.ctx, "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("alice holds %d subscriptions, want 2 (one replaced, one new)", len(subs))
	}

	// Every mutation lands on the log (S15.2 rule 3), and the endpoint never
	// does: three enrolments, three rows, all naming the subscription by hash.
	rows := e.events(push.EventSubscribed)
	if len(rows) != 3 {
		t.Fatalf("push.subscribed rows = %d, want 3", len(rows))
	}
	if rows[1]["replaced"] != true {
		t.Error("the replacement's audit row does not record that it replaced")
	}
	for i, r := range rows {
		if r["_owner"] != "alice" {
			t.Errorf("row %d attributed to %v", i, r["_owner"])
		}
		if _, ok := r["endpoint"]; ok {
			t.Errorf("row %d carries a raw endpoint", i)
		}
		if h, _ := r["endpoint_hash"].(string); len(h) != 64 {
			t.Errorf("row %d endpoint_hash = %q, want a sha256 hex", i, h)
		}
	}
}

// TestReadsAreOwnerScopedThreeWays: the operator sees every row, a member sees
// their own, and an unknown identity matches nothing. All three over ONE world,
// which is what makes "returns nothing" and "was never scoped" distinguishable.
func TestReadsAreOwnerScopedThreeWays(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct{ owner, endpoint string }{
		{"alice", "https://web.push.apple.com/alice-phone"},
		{"alice", "https://fcm.googleapis.com/alice-tablet"},
		{"bob", "https://web.push.apple.com/bob-phone"},
	} {
		if _, _, err := e.store.Enrol(e.ctx, c.owner, enrolment(t, c.endpoint)); err != nil {
			t.Fatalf("enrol %s: %v", c.owner, err)
		}
	}

	all, err := e.store.ListAll(e.ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("the operator's reading holds %d rows, want 3", len(all))
	}
	mine, err := e.store.List(e.ctx, "alice")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(mine) != 2 {
		t.Errorf("alice reads %d rows, want her own 2", len(mine))
	}
	for _, s := range mine {
		if s.UserID != "alice" {
			t.Errorf("alice's reading carries %s's row", s.UserID)
		}
	}
	nobody, err := e.store.List(e.ctx, "carol")
	if err != nil {
		t.Fatalf("List(carol): %v", err)
	}
	if len(nobody) != 0 {
		t.Errorf("an identity with no devices matched %d rows", len(nobody))
	}

	owners, err := e.store.Owners(e.ctx)
	if err != nil {
		t.Fatalf("Owners: %v", err)
	}
	if len(owners) != 2 || owners[0] != "alice" || owners[1] != "bob" {
		t.Errorf("Owners = %v, want [alice bob]", owners)
	}
}

// TestMetadataNeverCarriesTheEndpoint pins the OQ2 disposition structurally: the
// operator sees rows AS METADATA. An endpoint is a capability URL — anyone
// holding it can push to that device — so no read surface serves one, including
// the operator's.
func TestMetadataNeverCarriesTheEndpoint(t *testing.T) {
	e := newEnv(t)
	const endpoint = "https://web.push.apple.com/secret-capability-path"
	if _, _, err := e.store.Enrol(e.ctx, "alice", enrolment(t, endpoint)); err != nil {
		t.Fatalf("enrol: %v", err)
	}
	subs, err := e.store.ListAll(e.ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if subs[0].Endpoint != endpoint {
		t.Fatalf("the STORED row lost its endpoint: %q", subs[0].Endpoint)
	}
	raw, err := json.Marshal(subs[0].Metadata())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, endpoint) || strings.Contains(body, "secret-capability-path") {
		t.Errorf("Metadata serialized the endpoint: %s", body)
	}
	if strings.Contains(body, subs[0].Keys.P256DH) || strings.Contains(body, subs[0].Keys.Auth) {
		t.Errorf("Metadata serialized subscription key material: %s", body)
	}
	if !strings.Contains(body, push.HashEndpoint(endpoint)) {
		t.Errorf("Metadata does not name the subscription by its hash: %s", body)
	}
	// The non-tautological control: the hash really is derived from THIS
	// endpoint, so the assertion above is about identity and not about a
	// constant that happens to be present.
	if push.HashEndpoint(endpoint) == push.HashEndpoint(endpoint+"x") {
		t.Error("HashEndpoint does not distinguish two endpoints")
	}
}

// TestRemoveIsIdempotentAndOwnerScoped: removing twice is applied:false rather
// than an error (a phone retry is safe), and one person cannot remove another's
// device.
func TestRemoveIsIdempotentAndOwnerScoped(t *testing.T) {
	e := newEnv(t)
	const endpoint = "https://web.push.apple.com/alice-phone"
	if _, _, err := e.store.Enrol(e.ctx, "alice", enrolment(t, endpoint)); err != nil {
		t.Fatalf("enrol: %v", err)
	}

	if applied, err := e.store.Remove(e.ctx, "bob", endpoint); err != nil || applied {
		t.Errorf("bob removed alice's device: applied=%v err=%v", applied, err)
	}
	if subs, _ := e.store.List(e.ctx, "alice"); len(subs) != 1 {
		t.Fatal("a cross-owner remove took the row")
	}
	if applied, err := e.store.Remove(e.ctx, "alice", endpoint); err != nil || !applied {
		t.Errorf("the owner's remove: applied=%v err=%v", applied, err)
	}
	if applied, err := e.store.Remove(e.ctx, "alice", endpoint); err != nil || applied {
		t.Errorf("a repeated remove: applied=%v err=%v, want applied=false with no error", applied, err)
	}
	rows := e.events(push.EventUnsubscribed)
	if len(rows) != 1 {
		t.Fatalf("push.unsubscribed rows = %d, want exactly 1 (the repeat mints nothing)", len(rows))
	}
	if rows[0]["reason"] != push.ReasonOwnerUnenrolled {
		t.Errorf("reason = %v, want the person's own act — and NOT the D10 role word, which is what a member unenrolling their own phone used to record", rows[0]["reason"])
	}
	if push.ReasonOwnerUnenrolled == "operator" {
		t.Error("the self-service reason is the role word again")
	}
}

// TestEnrolmentBoundaryChecks covers the one boundary this package validates.
// Each case would otherwise become an outbound request target or a deep-link
// prefix, which is why each is refused at enrolment rather than discovered at
// send time when nobody is watching.
func TestEnrolmentBoundaryChecks(t *testing.T) {
	e := newEnv(t)
	good := enrolment(t, "https://web.push.apple.com/ok")
	cases := []struct {
		name string
		mut  func(push.Enrolment) push.Enrolment
	}{
		{"empty endpoint", func(x push.Enrolment) push.Enrolment { x.Endpoint = ""; return x }},
		{"http endpoint", func(x push.Enrolment) push.Enrolment { x.Endpoint = "http://push.example/x"; return x }},
		{"endpoint is not a URL", func(x push.Enrolment) push.Enrolment { x.Endpoint = "://"; return x }},
		{"oversized endpoint", func(x push.Enrolment) push.Enrolment {
			x.Endpoint = "https://push.example/" + strings.Repeat("a", push.MaxEndpointBytes)
			return x
		}},
		{"unusable p256dh", func(x push.Enrolment) push.Enrolment { x.Keys.P256DH = "nope"; return x }},
		{"unusable auth", func(x push.Enrolment) push.Enrolment { x.Keys.Auth = "nope"; return x }},
		{"empty origin", func(x push.Enrolment) push.Enrolment { x.Origin = ""; return x }},
		{"http origin off loopback", func(x push.Enrolment) push.Enrolment { x.Origin = "http://sinet.example"; return x }},
		{"origin with no host", func(x push.Enrolment) push.Enrolment { x.Origin = "https:///nope"; return x }},
		{"oversized label", func(x push.Enrolment) push.Enrolment {
			x.Label = strings.Repeat("l", push.MaxLabelRunes+1)
			return x
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := e.store.Enrol(e.ctx, "alice", c.mut(good)); err == nil {
				t.Fatal("accepted")
			} else if !errors.Is(err, push.ErrBadEnrolment) {
				t.Errorf("error %v does not classify as ErrBadEnrolment", err)
			}
		})
	}
	if _, _, err := e.store.Enrol(e.ctx, "alice", good); err != nil {
		t.Errorf("the control case failed: %v", err)
	}
	if _, _, err := e.store.Enrol(e.ctx, "", good); err == nil {
		t.Error("an enrolment with no identity was accepted")
	}
	// The origin is stored as an ORIGIN, path and query stripped: it becomes the
	// PREFIX of every deep link this device is sent.
	withPath := good
	withPath.Endpoint = "https://web.push.apple.com/origin-check"
	withPath.Origin = "https://sinet.example.ts.net/settings?view=push"
	sub, _, err := e.store.Enrol(e.ctx, "alice", withPath)
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	if sub.Origin != "https://sinet.example.ts.net" {
		t.Errorf("stored origin = %q, want the bare origin", sub.Origin)
	}
	// Loopback http IS admitted: it is a secure context, and the dev posture
	// serves plain HTTP there.
	local := good
	local.Endpoint = "https://web.push.apple.com/local"
	local.Origin = "http://127.0.0.1:5173"
	if _, _, err := e.store.Enrol(e.ctx, "alice", local); err != nil {
		t.Errorf("a loopback origin was refused: %v", err)
	}
}

// TestSubscriptionsPerOwnerIsBounded drives the bound and its per-owner scope.
func TestSubscriptionsPerOwnerIsBounded(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < push.MaxSubscriptionsPerUser; i++ {
		if _, _, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/d"+string(rune('a'+i)))); err != nil {
			t.Fatalf("enrol %d: %v", i, err)
		}
	}
	_, _, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/one-too-many"))
	if err == nil {
		t.Fatal("the bound did not refuse")
	}
	if !strings.Contains(err.Error(), "20") {
		t.Errorf("the refusal %q does not name its bound", err)
	}
	// A REPLACEMENT is not a new device and must not be refused by a bound it
	// does not consume (the internal/chat dedupe precedent).
	if _, replaced, err := e.store.Enrol(e.ctx, "alice", enrolment(t, "https://web.push.apple.com/da")); err != nil || !replaced {
		t.Errorf("a re-enrol at the bound was refused: replaced=%v err=%v", replaced, err)
	}
	// The bound is PER OWNER.
	if _, _, err := e.store.Enrol(e.ctx, "bob", enrolment(t, "https://web.push.apple.com/bob")); err != nil {
		t.Errorf("bob's first device was refused by alice's bound: %v", err)
	}
}

// TestVAPIDKeyIsHouseholdStableAndPrivate: the key is minted once, survives a
// fresh store over the same state directory (a restart), and is written 0600.
// Stability is the load-bearing half — every browser subscribes with the public
// key, so a key that changed per process would silently invalidate every
// subscription in the household.
func TestVAPIDKeyIsHouseholdStableAndPrivate(t *testing.T) {
	e := newEnv(t)
	first := e.store.PublicKey()
	if first == "" {
		t.Fatal("no VAPID public key")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(raw) != 65 || raw[0] != 0x04 {
		t.Fatalf("public key is not a 65-octet uncompressed point: %v (%d bytes)", err, len(raw))
	}

	again := e.open().PublicKey()
	if again != first {
		t.Errorf("a fresh store over the same state dir minted a DIFFERENT key:\n %s\n %s", first, again)
	}

	info, err := os.Stat(filepath.Join(e.stateDir, "vapid-key.pem"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("VAPID key mode = %o, want 600", perm)
	}
	// The control: a DIFFERENT state directory is a different household and
	// gets its own key, so the stability assertion above is about persistence
	// and not about a constant.
	other, err := push.New(push.Config{DB: e.db, Log: e.log, StateDir: filepath.Join(t.TempDir(), "other")})
	if err != nil {
		t.Fatalf("push.New: %v", err)
	}
	if other.PublicKey() == first {
		t.Error("two independent state directories produced the same VAPID key")
	}
}
