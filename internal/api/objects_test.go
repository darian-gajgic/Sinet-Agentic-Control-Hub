package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// objects_test.go — the B6-8 OQ2b object-bytes read.
//
// The read exists because the S13.2 image trio and the binary card's
// download-to-inspect need content and `ObjectRef` carries only metadata plus a
// hash. Everything asserted here is a wall rather than a feature: the sha is not
// an oracle, the scope is the deliverable's, and served bytes never become a
// second raw-HTML channel.

// objectEnv mints one image deliverable and one opaque one, both with recorded
// object types, and hands back the shas so a test can name a REAL pinned object
// rather than a hash it invented.
type objectEnv struct {
	e        *dlvEnv
	imageSHA string
	blobSHA  string
	svgSHA   string
}

func newObjectEnv(t *testing.T) objectEnv {
	t.Helper()
	e := newDlvEnv(t)
	e.mkRun("t-obj", "r-obj", "alice")
	e.mkRun("t-obj-bob", "r-obj-bob", "bob")

	mint := func(id, owner, task, runID, dtype, name string, body []byte, recorded string) string {
		t.Helper()
		if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
			ID: id, Owner: owner, TaskID: task, Type: dtype,
		}); err != nil {
			t.Fatalf("EnsureDeliverable %s: %v", id, err)
		}
		rev, err := e.rev.MintRevision(e.ctx, review.MintInput{
			DeliverableID: id, N: 1, RunID: runID,
			Objects: map[string][]byte{name: body},
			Types:   map[string]string{name: recorded},
		})
		if err != nil {
			t.Fatalf("MintRevision %s: %v", id, err)
		}
		if len(rev.Objects) != 1 {
			t.Fatalf("%s minted %d objects, want 1", id, len(rev.Objects))
		}
		return rev.Objects[0].SHA256
	}

	return objectEnv{
		e:        e,
		imageSHA: mint("d-shot", "alice", "t-obj", "r-obj", "image", "shot.png", []byte("\x89PNG\r\n\x1a\nPIXELS"), "image/png"),
		blobSHA:  mint("d-blob", "alice", "t-obj", "r-obj", "binary", "dist/bundle.tar", []byte("tarbytes"), "application/x-tar"),
		// A scriptable document wearing an image type. It is pinned so the
		// attachment lane can be asserted on the ONE case where the inline lane
		// would be a raw-markup channel.
		svgSHA: mint("d-vector", "alice", "t-obj", "r-obj", "image", "logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml"),
	}
}

func (o objectEnv) get(t *testing.T, who, deliverable, sha string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	o.e.server(who).Handler().ServeHTTP(rr,
		httptest.NewRequest("GET", "/api/deliverables/"+deliverable+"/objects/"+sha, nil))
	return rr
}

// TestObjectBytesServeTheRevisionsOwnPinnedContent is the read working: the
// bytes come back, they are the bytes that were minted, and the hash in the path
// is what selected them.
func TestObjectBytesServeTheRevisionsOwnPinnedContent(t *testing.T) {
	o := newObjectEnv(t)
	rr := o.get(t, "alice", "d-shot", o.imageSHA)
	if rr.Code != http.StatusOK {
		t.Fatalf("read own pinned object: %d %s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != "\x89PNG\r\n\x1a\nPIXELS" {
		t.Errorf("served bytes are not the minted bytes: %q", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "14" {
		t.Errorf("Content-Length must be the ref's recorded size, got %q", got)
	}
}

// TestObjectBytesAreNotAShaOracle is the wall the route exists behind: a hash
// that is REAL — it is in the object dir, pinned by another deliverable — is
// still 404 here, because this deliverable's revisions do not pin it. Without
// this, the route would answer differently for a sha that exists somewhere,
// which is exactly the existence oracle 404-before-403 exists to close.
func TestObjectBytesAreNotAShaOracle(t *testing.T) {
	o := newObjectEnv(t)
	// The non-tautological control: the same sha READS on the deliverable that
	// pins it, so the 404 below is about scope and not about a bad hash.
	if rr := o.get(t, "alice", "d-blob", o.blobSHA); rr.Code != http.StatusOK {
		t.Fatalf("control: the pinning deliverable must serve its own object: %d", rr.Code)
	}
	if rr := o.get(t, "alice", "d-shot", o.blobSHA); rr.Code != http.StatusNotFound {
		t.Errorf("a sha pinned by a DIFFERENT deliverable must be 404 here, got %d %s", rr.Code, rr.Body.String())
	}
	if rr := o.get(t, "alice", "d-shot", strings.Repeat("ab", 32)); rr.Code != http.StatusNotFound {
		t.Errorf("an unknown sha must be 404, got %d", rr.Code)
	}
}

// TestObjectBytesResolveTheDeliverableScopeThreeWays is the standard owner-scope
// walk, with 404 before 403 on the deliverable itself.
func TestObjectBytesResolveTheDeliverableScopeThreeWays(t *testing.T) {
	o := newObjectEnv(t)
	for _, tc := range []struct{ who, want string }{
		{"alice", "owner reads their own"},
		{"op", "the operator reads anyone's"},
	} {
		if rr := o.get(t, tc.who, "d-shot", o.imageSHA); rr.Code != http.StatusOK {
			t.Errorf("%s: %d %s", tc.want, rr.Code, rr.Body.String())
		}
	}
	// A co-member is refused, through the SAME `deliverableScope` every other
	// read in the family goes through: 404-before-403 means the not-found branch
	// runs FIRST, so an unknown id cannot fall into the ownership check and come
	// back as a 403 that confirms the id exists. An id that DOES exist and is not
	// yours is a 403 — the landed answer, asserted as it is rather than as a
	// stronger property than the platform has.
	if rr := o.get(t, "bob", "d-shot", o.imageSHA); rr.Code != http.StatusForbidden {
		t.Errorf("another member must be refused, got %d %s", rr.Code, rr.Body.String())
	}
	if unknown := o.get(t, "bob", "d-nope", o.imageSHA); unknown.Code != http.StatusNotFound {
		t.Errorf("an unknown deliverable must be 404 and never a 403, got %d", unknown.Code)
	}
}

// TestObjectBytesNeverBecomeASecondRawHTMLChannel is the OQ2b inline line, as
// assertions. Four raster image types render inline because the S15.8 trio needs
// them to; everything else — INCLUDING an SVG, which is a scriptable document
// wearing an image type — is an octet-stream attachment.
func TestObjectBytesNeverBecomeASecondRawHTMLChannel(t *testing.T) {
	o := newObjectEnv(t)

	inline := o.get(t, "alice", "d-shot", o.imageSHA)
	if got := inline.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("a recorded image/png must serve inline as image/png, got %q", got)
	}
	if got := inline.Header().Get("Content-Disposition"); got != "" {
		t.Errorf("the inline lane carries no disposition, got %q", got)
	}

	for _, tc := range []struct{ deliverable, sha, why string }{
		{"d-blob", o.blobSHA, "an opaque type"},
		{"d-vector", o.svgSHA, "an SVG — scriptable markup wearing an image type"},
	} {
		rr := o.get(t, "alice", tc.deliverable, tc.sha)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", tc.why, rr.Code, rr.Body.String())
		}
		if got := rr.Header().Get("Content-Type"); got != "application/octet-stream" {
			t.Errorf("%s must serve as application/octet-stream, got %q", tc.why, got)
		}
		if got := rr.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
			t.Errorf("%s must be an attachment, got %q", tc.why, got)
		}
	}

	// The disposition offers the last path element only, encoded by the stdlib
	// rather than hand-quoted: the name is a producer-supplied path.
	if got := o.get(t, "alice", "d-blob", o.blobSHA).Header().Get("Content-Disposition"); !strings.Contains(got, "bundle.tar") ||
		strings.Contains(got, "dist/") {
		t.Errorf("the disposition must name the base filename only, got %q", got)
	}

	// The header set, on BOTH lanes: the inline lane cannot rely on a disposition,
	// so nosniff and the sandbox CSP are what keep a navigation to these bytes
	// from becoming a document.
	for _, sha := range []string{o.imageSHA} {
		h := o.get(t, "alice", "d-shot", sha).Header()
		for header, want := range map[string]string{
			"X-Content-Type-Options":       "nosniff",
			"Content-Security-Policy":      "default-src 'none'; sandbox",
			"Cross-Origin-Resource-Policy": "same-origin",
			"X-Frame-Options":              "DENY",
		} {
			if got := h.Get(header); got != want {
				t.Errorf("%s: want %q, got %q", header, want, got)
			}
		}
	}
	blob := o.get(t, "alice", "d-blob", o.blobSHA).Header()
	if blob.Get("X-Content-Type-Options") != "nosniff" || blob.Get("Content-Security-Policy") == "" {
		t.Errorf("the attachment lane keeps the same headers: %+v", blob)
	}
}

// TestObjectBytesRefuseADriftedObjectInsteadOfServingIt is drain r2 R9, and it is
// what the D10 drift test became.
//
// D10 fixed the LENGTH (it came from the recorded size while the body came from the
// file, so the two could disagree) and left the referral: the route still streamed
// without the hash re-check `review.Store.readObject` performs, so a drifted object
// was answered 200 with a now-plausible Content-Length. A URL that CONTAINS the hash
// of its own answer cannot do that — it is the response contradicting its own
// identity — so the bytes are verified before any of them is written, and drift
// takes the same `content_drift` posture the missing-bytes case already took.
func TestObjectBytesRefuseADriftedObjectInsteadOfServingIt(t *testing.T) {
	o := newObjectEnv(t)
	// The ROW cannot drift — a minted revision is immutable by trigger, which is
	// the store protecting its own record. So the drift is where it really happens:
	// the bytes on disk move underneath a hash that still names the old ones.
	path := o.e.rev.ObjectPath(o.imageSHA)
	drifted := []byte("\x89PNG\r\n\x1a\nPIXELS-AND-MORE")
	if err := os.WriteFile(path, drifted, 0o600); err != nil {
		t.Fatalf("drift the object bytes: %v", err)
	}

	rr := o.get(t, "alice", "d-shot", o.imageSHA)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("a drifted object must be refused, got %d %s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "content_drift") {
		t.Errorf("the refusal must carry the platform's own content_drift code, got %s", body)
	}
	// And not one drifted byte reached the caller alongside the refusal.
	if body := rr.Body.String(); strings.Contains(body, "PIXELS-AND-MORE") {
		t.Errorf("drifted bytes were written before the refusal: %q", body)
	}
	// The sibling object is untouched, so the refusal is about THESE bytes and not
	// a route that stopped working.
	if ok := o.get(t, "alice", "d-blob", o.blobSHA); ok.Code != http.StatusOK {
		t.Errorf("an intact object must still serve: %d %s", ok.Code, ok.Body.String())
	}
}

// TestObjectBytesLengthComesFromTheFileNotTheRecordedSize keeps D10's guarantee now
// that a drifted object is refused rather than served, which is what D10 used to
// drive it with.
//
// The length still must not come from `ref.Size`: it comes from the file, through
// http.ServeContent, which is also what answers HEAD and Range by construction. A
// RANGE read is the non-tautological driver — a handler that set Content-Length from
// the recorded size and copied the file would answer 200 with the whole body, so
// only arithmetic done over what is actually being served can produce this answer.
func TestObjectBytesLengthComesFromTheFileNotTheRecordedSize(t *testing.T) {
	o := newObjectEnv(t)
	full := "\x89PNG\r\n\x1a\nPIXELS"

	req := httptest.NewRequest("GET", "/api/deliverables/d-shot/objects/"+o.imageSHA, nil)
	req.Header.Set("Range", "bytes=8-13")
	rr := httptest.NewRecorder()
	o.e.server("alice").Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusPartialContent {
		t.Fatalf("a range read must be answered from the file: %d %s", rr.Code, rr.Body.String())
	}
	if got, want := rr.Body.String(), full[8:14]; got != want {
		t.Errorf("the range body is %q, want %q", got, want)
	}
	if got := rr.Header().Get("Content-Length"); got != "6" {
		t.Errorf("Content-Length must be the SERVED length, got %q", got)
	}
	if got, want := rr.Header().Get("Content-Range"), "bytes 8-13/"+strconv.Itoa(len(full)); got != want {
		t.Errorf("Content-Range %q, want %q", got, want)
	}

	// And a HEAD carries the whole file's length with no body — the same machinery,
	// asserted so "ServeContent handles it" is checked rather than assumed.
	head := httptest.NewRequest("HEAD", "/api/deliverables/d-shot/objects/"+o.imageSHA, nil)
	hr := httptest.NewRecorder()
	o.e.server("alice").Handler().ServeHTTP(hr, head)
	if hr.Code != http.StatusOK || hr.Body.Len() != 0 {
		t.Errorf("HEAD: %d with a %d-byte body", hr.Code, hr.Body.Len())
	}
	if got := hr.Header().Get("Content-Length"); got != strconv.Itoa(len(full)) {
		t.Errorf("HEAD Content-Length %q, want %d", got, len(full))
	}
}

// TestObjectBytesAreCachedByTheirOwnHash pins the content-addressed posture: the
// sha in the path is the hash of the bytes, so the answer at a URL can never
// change — and `private` keeps a shared cache out of an owner-scoped read.
func TestObjectBytesAreCachedByTheirOwnHash(t *testing.T) {
	o := newObjectEnv(t)
	got := o.get(t, "alice", "d-shot", o.imageSHA).Header().Get("Cache-Control")
	for _, want := range []string{"private", "immutable"} {
		if !strings.Contains(got, want) {
			t.Errorf("Cache-Control %q is missing %q", got, want)
		}
	}
}

// The not-wired, session-required and unversioned answers are asserted by the
// landed walks in deliverables_test.go, whose `deliverablesRoutes` inventory this
// route joined — one enumeration, so a route added without those answers fails
// there rather than needing a copy of the walk here.
