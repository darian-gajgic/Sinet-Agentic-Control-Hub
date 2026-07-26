package watchlist_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/lockfile"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// cdio_test.go — the changedetection.io driver battery. Every call goes to an
// httptest STUB serving the route shapes read from the pinned source at tag
// 0.55.8; nothing dials a live host and nothing is installed (the B5-5
// promptfoo precedent). The real leg is the sanctioned skip at the bottom.

const stubAPIKey = "test-key"

// organStub is the hermetic changedetection.io stand-in. It implements the
// pinned surface faithfully: the list is an OBJECT keyed by uuid carrying the
// nine-key allowlist, create is 201 + {"uuid":…}, PUT is 200 "OK", DELETE is
// 204, difference is text/plain, and a bad x-api-key is 403.
type organStub struct {
	mu       sync.Mutex
	watches  map[string]map[string]any
	tags     map[string]map[string]any
	diff     string
	created  int
	updated  int
	deleted  int
	nextID   int
	version  string
	rejected bool
}

func newOrganStub() *organStub {
	return &organStub{
		watches: map[string]map[string]any{},
		tags:    map[string]map[string]any{},
		diff:    "- Sonnet $3/Mtok\n+ Sonnet $4/Mtok\n",
		version: watchlist.Pin,
	}
}

func (s *organStub) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != stubAPIKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`"Invalid access - API key invalid."`))
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/api/v1/systeminfo", auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"queue_size": 0, "overdue_watches": []string{}, "uptime": 12.5,
			"watch_count": len(s.watches), "version": s.version,
		})
	}))
	mux.HandleFunc("/api/v1/tags", auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, s.tags)
	}))
	mux.HandleFunc("/api/v1/tag", auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.nextID++
		id := fmt.Sprintf("tag-%d", s.nextID)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.tags[id] = map[string]any{"title": body["title"], "uuid": id}
		writeJSON(w, http.StatusCreated, map[string]string{"uuid": id})
	}))
	mux.HandleFunc("/api/v1/watch", auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, s.watches)
		case http.MethodPost:
			var spec map[string]any
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				http.Error(w, "Invalid or unsupported URL", http.StatusBadRequest)
				return
			}
			s.nextID++
			id := fmt.Sprintf("uuid-%d", s.nextID)
			var tagIDs []string
			if title, ok := spec["tag"].(string); ok && title != "" {
				for tid, tg := range s.tags {
					if tg["title"] == title {
						tagIDs = append(tagIDs, tid)
					}
				}
			}
			s.watches[id] = map[string]any{
				"url": spec["url"], "title": spec["title"], "tags": tagIDs,
				"last_changed": 0, "last_checked": 0, "last_error": nil,
				"link": spec["url"], "page_title": spec["title"], "viewed": false,
			}
			s.created++
			writeJSON(w, http.StatusCreated, map[string]string{"uuid": id})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/watch/", auth(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/watch/")
		id, sub, _ := strings.Cut(rest, "/")
		if strings.HasPrefix(sub, "difference/") {
			if _, ok := s.watches[id]; !ok {
				http.Error(w, "No watch exists", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(s.diff))
			return
		}
		switch r.Method {
		case http.MethodPut:
			if _, ok := s.watches[id]; !ok {
				http.Error(w, "No watch exists", http.StatusNotFound)
				return
			}
			var spec map[string]any
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			// The pinned PUT REJECTS unknown fields; the stub mirrors that so a
			// driver sending an off-schema key fails here, not in production.
			for k := range spec {
				switch k {
				case "url", "title", "llm_intent", "fetch_backend":
				default:
					http.Error(w, "Unknown field(s): "+k, http.StatusBadRequest)
					return
				}
			}
			s.watches[id]["url"] = spec["url"]
			s.updated++
			_, _ = w.Write([]byte("OK"))
		case http.MethodDelete:
			if _, ok := s.watches[id]; !ok {
				http.Error(w, "No watch exists", http.StatusBadRequest)
				return
			}
			delete(s.watches, id)
			s.deleted++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func pageRow(id, url string) watchlist.Row {
	return watchlist.Row{
		ID: id, Kind: watchlist.KindPage, URL: url, Lane: "anthropic", Tier: 1,
		Cadence: watchlist.CadenceWeekly, Enabled: true, Group: watchlist.GroupReport02,
		Notes: "test page row", MigrateBias: true,
	}
}

// TestReconcileIsConfigAsCodeAndIdempotent (R6): Sinet's rows are the source of
// truth. The first pass creates, the second updates in place (no duplicate —
// the pinned create endpoint does NOT detect duplicate urls, so re-binding by
// external ref and by url is what keeps a restart from growing the watch set),
// and a Sinet-tagged remote watch with no matching row is deleted.
func TestReconcileIsConfigAsCodeAndIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	stub := newOrganStub()
	srv := stub.serve(t)
	c := watchlist.NewCDIO(srv.URL, stubAPIKey, srv.Client())

	h.seedOne(t, pageRow("p1", "https://example.invalid/pricing"))
	rows, err := h.st.PageRows(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := c.Reconcile(ctx, h.st, rows)
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if rep.Created != 1 || rep.Updated != 0 {
		t.Fatalf("first reconcile = %+v, want one create", rep)
	}
	got := h.row(t, "p1")
	if got.ExternalRef == "" {
		t.Fatal("the organ watch uuid was not persisted on the row — reconciliation would not be idempotent")
	}

	rows, _ = h.st.PageRows(ctx)
	rep, err = c.Reconcile(ctx, h.st, rows)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if rep.Created != 0 || rep.Updated != 1 {
		t.Errorf("second reconcile = %+v, want one update and no create", rep)
	}
	if len(stub.watches) != 1 {
		t.Errorf("the organ carries %d watches after two passes, want 1", len(stub.watches))
	}

	// A hand-edit in the organ's UI — an extra Sinet-tagged watch — is
	// reconciled away, never honored.
	stub.mu.Lock()
	stub.watches["rogue"] = map[string]any{"url": "https://hand-edited.invalid/", "tags": []string{rep.TagID}}
	stub.mu.Unlock()
	rows, _ = h.st.PageRows(ctx)
	rep, err = c.Reconcile(ctx, h.st, rows)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 1 {
		t.Errorf("a Sinet-tagged watch with no row was not deleted (%+v)", rep)
	}

	// A watch the OPERATOR created outside Sinet's tag is left alone.
	stub.mu.Lock()
	stub.watches["operators-own"] = map[string]any{"url": "https://mine.invalid/", "tags": []string{}}
	stub.mu.Unlock()
	rows, _ = h.st.PageRows(ctx)
	if _, err := c.Reconcile(ctx, h.st, rows); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	_, stillThere := stub.watches["operators-own"]
	stub.mu.Unlock()
	if !stillThere {
		t.Error("reconciliation deleted a watch outside Sinet's tag — it owns only its own")
	}
}

// TestEscalatedRowRequestsTheBrowserBackend: the decay ladder's second rung is
// expressed as CONFIGURATION on the reconciled watch. Whether a browser leg
// exists is the organ's fact (at 0.55.8 it is an external CDP service selected
// by PLAYWRIGHT_DRIVER_URL), so Sinet asks and never claims.
func TestEscalatedRowRequestsTheBrowserBackend(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	stub := newOrganStub()
	srv := stub.serve(t)

	var seen string
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/watch" {
			body, _ := json.Marshal(map[string]string{})
			_ = body
			var spec map[string]any
			_ = json.NewDecoder(r.Body).Decode(&spec)
			if b, ok := spec["fetch_backend"].(string); ok {
				seen = b
			}
			writeJSON(w, http.StatusCreated, map[string]string{"uuid": "u1"})
			return
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer probe.Close()

	c := watchlist.NewCDIO(probe.URL, stubAPIKey, probe.Client())
	h.seedOne(t, pageRow("p2", "https://example.invalid/plans"))
	if err := h.st.SetDecayStage(ctx, "p2", watchlist.DecayEscalated); err != nil {
		t.Fatal(err)
	}
	rows, _ := h.st.PageRows(ctx)
	if _, err := c.Reconcile(ctx, h.st, rows); err != nil {
		t.Fatal(err)
	}
	if seen != watchlist.FetchBackendBrowser {
		t.Errorf("an escalated row requested fetch_backend %q, want %q", seen, watchlist.FetchBackendBrowser)
	}
}

// TestAuthFailureIsRefused: a bad key is 403 at the pinned tag and surfaces as
// an error rather than an empty result set.
func TestAuthFailureIsRefused(t *testing.T) {
	srv := newOrganStub().serve(t)
	c := watchlist.NewCDIO(srv.URL, "wrong-key", srv.Client())
	if _, err := c.SystemInfo(context.Background()); err == nil {
		t.Error("a bad x-api-key was not refused")
	} else if !strings.Contains(err.Error(), "403") {
		t.Errorf("auth failure surfaced as %v, want a 403", err)
	}
}

// TestOrganLivenessReportsPinDeltaLoudly (CONVENTIONS §10): an installed-vs-pin
// delta is reported, never silently retargeted, and an unreachable or
// unconfigured organ is honestly DOWN.
func TestOrganLivenessReportsPinDeltaLoudly(t *testing.T) {
	h := newHarness(t)
	stub := newOrganStub()
	srv := stub.serve(t)

	x := watchlist.New(watchlist.Deps{
		Store: h.st, Emitter: h.em, Settings: h.reg,
		CDIO: watchlist.NewCDIO(srv.URL, stubAPIKey, srv.Client()),
	})
	up, note := x.OrganLiveness(context.Background())
	if !up || !strings.Contains(note, watchlist.Pin) {
		t.Errorf("at the pin: up=%v note=%q", up, note)
	}

	stub.mu.Lock()
	stub.version = "0.56.0"
	stub.mu.Unlock()
	up, note = x.OrganLiveness(context.Background())
	if !up || !strings.Contains(note, "PIN DELTA") {
		t.Errorf("a pin delta must be reported LOUDLY: up=%v note=%q", up, note)
	}

	absent := watchlist.New(watchlist.Deps{Store: h.st, Emitter: h.em, Settings: h.reg})
	if up, note := absent.OrganLiveness(context.Background()); up || !strings.Contains(note, "not configured") {
		t.Errorf("an unconfigured organ must be DOWN and say so: up=%v note=%q", up, note)
	}
}

// TestFeedAndAPITiersWorkWithThePageTierAbsent is acceptance item 15: an absent
// organ degrades the page tier alone.
func TestFeedAndAPITiersWorkWithThePageTierAbsent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	feedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(atomBody))
	}))
	defer feedSrv.Close()

	h.seedOne(t, feedRow("f1", feedSrv.URL))
	h.seedOne(t, pageRow("p1", "https://example.invalid/pricing"))

	// No CDIO configured at all: the page tier is absent.
	x := watchlist.New(watchlist.Deps{
		Store: h.st, Emitter: h.em, Settings: h.reg, HTTPClient: feedSrv.Client(),
	})
	pass, err := x.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if pass.Hits != 2 {
		t.Errorf("the feed tier produced %d hits, want 2 — it must keep working without the organ", pass.Hits)
	}
	if pass.Failures != 1 {
		t.Errorf("the page row should have failed honestly (organ absent), failures=%d", pass.Failures)
	}
}

// TestPinMatchesLock couples the exported pin to the components.lock entry, so
// a bump is one deliberate edit in two places rather than a silent drift.
func TestPinMatchesLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "components.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lf, err := lockfile.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lf.Components {
		if c.Name == "changedetection.io" {
			if c.Pin != watchlist.Pin {
				t.Errorf("components.lock pins changedetection.io %q, the package pins %q", c.Pin, watchlist.Pin)
			}
			if c.Kind != "organ-unit" {
				t.Errorf("changedetection.io kind = %q, want organ-unit", c.Kind)
			}
			if len(c.Modules) != 0 {
				t.Error("changedetection.io must claim no Go modules — it is a Python service")
			}
			return
		}
	}
	t.Fatal("components.lock carries no changedetection.io entry")
}

// TestRealOrganLegOrSanctionedSkip is the CONVENTIONS §10 real leg: it runs only
// when an operator has actually installed and pointed at the organ, and prints
// the one sanctioned skip otherwise. It asserts BEHAVIOR (the live systeminfo
// version against the pin), never documentation.
func TestRealOrganLegOrSanctionedSkip(t *testing.T) {
	base := os.Getenv(watchlist.CDIOURLEnv)
	if base == "" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): " + watchlist.CDIOURLEnv +
			" is unset — changedetection.io is not installed. Its host install is a B5-gate operator act; this packet installs nothing.")
	}
	c := watchlist.DiscoverCDIO(nil)
	if c == nil {
		t.Fatalf("%s is set but discovery returned no client", watchlist.CDIOURLEnv)
	}
	info, err := c.SystemInfo(context.Background())
	if err != nil {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): the configured changedetection.io is unreachable: " + err.Error())
	}
	if info.Version != watchlist.Pin {
		t.Errorf("PIN DELTA: the installed changedetection.io reports %q, components.lock pins %q — report it, never retarget silently (CONVENTIONS §10)", info.Version, watchlist.Pin)
	}
}
