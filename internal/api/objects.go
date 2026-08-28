package api

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// objects.go — GET /api/deliverables/{deliverable}/objects/{sha} (P3-B6-8, OQ2b).
//
// THE S13.2 OBJECT SURFACES NEED BYTES, AND NOTHING SERVED THEM. `ObjectRef`
// carries a name, a size, a hash and a recorded type; the image-pair trio
// (2-up / swipe / onion-skin) and the binary card's "download to inspect" both
// need the content behind that hash. `review.Store.ObjectPath` is exported and
// no route reached it, so this read is the one addition — additive, owner-scoped,
// and no new verb on any other family.
//
// IT IS NOT A SHA ORACLE. The sha is resolved ONLY against the ObjectRefs of
// THIS deliverable's own revisions. An unknown-to-this-deliverable hash answers
// 404 even when the object dir holds it, so the route cannot be used to read
// another owner's content by guessing a hash — the 404-before-403 rule applied
// one level down, past the deliverable scope that already runs first.
//
// SERVED BYTES MUST NEVER BECOME A SECOND RAW-HTML CHANNEL. The one sanctioned
// raw-markup channel is the S15.8 preview iframe riding `src`
// (review.EscapeFirst); a deliverable's own bytes are content the platform did
// not author, so this route renders NOTHING as a document:
//
//   - Content-Type is set EXPLICITLY from a CLOSED allowlist, never sniffed and
//     never taken verbatim from the recorded label. Only four raster image types
//     are served inline, because that is what the S15.8 trio renders.
//     image/svg+xml is DELIBERATELY ABSENT: an SVG is a scriptable document that
//     merely looks like an image, so it takes the attachment lane with the rest.
//   - everything else is application/octet-stream + Content-Disposition:
//     attachment (RFC 6266) — a download, never a navigation target.
//   - X-Content-Type-Options: nosniff bars the Fetch-Standard sniffing path that
//     would otherwise let octet-stream bytes be treated as HTML.
//   - Content-Security-Policy: default-src 'none'; sandbox applies to the
//     response itself: navigated to directly it lands in an opaque origin with
//     scripts, forms and plugins disabled. Belt to nosniff's braces, because the
//     inline lane cannot rely on Content-Disposition.
//   - Cross-Origin-Resource-Policy: same-origin keeps a foreign origin from
//     embedding these bytes, and X-Frame-Options: DENY keeps the response out of
//     a frame. Neither affects <img>, which is how the trio reads them.
//
// AND THE BYTES ARE VERIFIED BEFORE ANY OF THEM IS WRITTEN (drain r2 R9). A route
// whose URL contains the hash of its own answer cannot serve bytes that hash to
// something else; that is the response contradicting its own identity, and it is
// the posture every in-process content read in internal/review already takes. The
// check is review.OpenObjectVerified — measured, constant-memory, over the same
// handle that is then streamed.

// inlineObjectTypes is the CLOSED set of recorded object labels served inline,
// mapped to the Content-Type actually sent. Two forms of each label are admitted
// because the recorded type is a producer's label rather than a wire header, and
// both spellings appear in practice; anything not listed here takes the
// attachment lane, which is the safe default rather than a guess.
//
// image/svg+xml is absent on purpose — see the file comment.
var inlineObjectTypes = map[string]string{
	"image/png":  "image/png",
	"png":        "image/png",
	"image/jpeg": "image/jpeg",
	"jpeg":       "image/jpeg",
	"jpg":        "image/jpeg",
	"image/gif":  "image/gif",
	"gif":        "image/gif",
	"image/webp": "image/webp",
	"webp":       "image/webp",
}

// objectCacheControl is the content-addressed cache posture. The sha in the path
// IS the hash of the bytes, so what a URL answers can never change — the same
// reason internal/webui serves hashed assets immutable (§41). `private` keeps a
// shared cache out of an owner-scoped read.
const objectCacheControl = "private, max-age=31536000, immutable"

func (s *Server) handleDeliverableObject(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	sha := r.PathValue("sha")
	revs, err := s.review.Revisions(r.Context(), d.ID)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	ref, ok := objectRefFor(revs, sha)
	if !ok {
		// 404 for a hash this deliverable does not pin, whether or not the object
		// dir holds it. The alternative would answer differently for a sha that
		// exists elsewhere, which is the existence oracle the scope rule bars.
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			// S13.2 content addressing: a revision pins objects by hash.
			Msg: fmt.Sprintf("no version of this work refers to a file with that fingerprint (%q)", sha)})
		return
	}

	// VERIFY BEFORE SERVE. The sha in the path IS the identity of the answer, so
	// bytes that hash differently are the response contradicting itself — the store's
	// own rule for every in-process read, applied to the streaming one. See
	// review.OpenObjectVerified for the cost measurement and the same-inode reason.
	f, info, err := s.review.OpenObjectVerified(ref.SHA256)
	if err == nil {
		defer f.Close()
	} else {
		switch {
		case errors.Is(err, os.ErrNotExist):
			// The row pins a hash whose bytes are gone. That is a platform
			// integrity failure, not something the caller can ask differently —
			// the same posture reviewErr takes on ErrContentDrift.
			s.logger.Error("objects: pinned object bytes are missing",
				"deliverable", d.ID, "sha", ref.SHA256, "err", err)
			s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "content_drift",
				Msg: "this revision pins an object whose bytes are not in the object dir (S13.1 retention)"})
		case errors.Is(err, review.ErrContentDrift):
			// The bytes are there and are NOT the bytes this URL names. Same
			// vocabulary as the missing case and as reviewErr's — one posture for
			// one class, rather than a second word for the same failure.
			s.logger.Error("objects: pinned object bytes do not match their hash",
				"deliverable", d.ID, "sha", ref.SHA256, "err", err)
			s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "content_drift",
				Msg: "this revision pins an object whose stored bytes hash differently (S13.2 content addressing)"})
		default:
			s.writeSurface(w, nil, fmt.Errorf("open object %s: %w", ref.SHA256, err))
		}
		return
	}

	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cache-Control", objectCacheControl)
	if ctype, inline := inlineObjectTypes[ref.Type]; inline {
		h.Set("Content-Type", ctype)
	} else {
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", objectDisposition(ref.Name))
	}
	// ServeContent sets Content-Length from the FILE, not from the recorded size,
	// and it is what keeps the two self-consistent: taking the length from the ref
	// while streaming the file would truncate or short-write whenever the two
	// disagree — which is exactly the drift this handler already refuses to serve
	// when the bytes are missing entirely. It also handles HEAD and range requests
	// by construction. The name is passed empty so it can never re-derive a
	// Content-Type from an extension: the type is decided above, from the closed
	// allowlist, and sniffing is the thing this route exists to prevent.
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// objectRefFor finds the pinned ref for a sha across every revision of one
// deliverable. The revisions are the authority on what this deliverable's bytes
// are; the object dir is only where they live.
func objectRefFor(revs []review.Revision, sha string) (review.ObjectRef, bool) {
	if sha == "" {
		return review.ObjectRef{}, false
	}
	for _, rev := range revs {
		for _, ref := range rev.Objects {
			if ref.SHA256 == sha {
				return ref, true
			}
		}
	}
	return review.ObjectRef{}, false
}

// objectDisposition renders the RFC 6266 attachment header. The name is a
// producer-supplied path, so only its last element is offered and the encoding
// is stdlib's rather than hand-quoted: a name carrying a quote or a non-ASCII
// character would otherwise compose a header field the caller reads differently
// from what was meant.
func objectDisposition(name string) string {
	base := path.Base(name)
	if base == "." || base == "/" || base == "" {
		base = "object"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": base})
}
